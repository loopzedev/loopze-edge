// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

func newTcpOutNode(t *testing.T, props map[string]any, reg *flow.SessionRegistry) *TCPOutNode {
	t.Helper()
	inst, err := NewTCPOutNode(flow.NodeConfig{
		ID:         "tcp-out-1",
		Type:       "tcp-out",
		Properties: props,
	})
	if err != nil {
		t.Fatal(err)
	}
	n := inst.(*TCPOutNode)
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if reg == nil {
		reg = flow.NewSessionRegistry()
	}
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(noopStatus)
	n.SetDebug(noopDebug)
	n.SetError(func(error, *flow.Message) {})
	n.SetSessionRegistry(reg)
	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })
	return n
}

// pairedSession registers a fake session via net.Pipe; the caller can
// read from peer to verify what the node wrote to slot.Conn.
func pairedSession(t *testing.T, reg *flow.SessionRegistry, owner string) (*flow.SessionHandle, net.Conn) {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})
	h, _ := reg.Register(server, owner)
	return h, client
}

func TestTcpOutReplyWritesOnSession(t *testing.T) {
	reg := flow.NewSessionRegistry()
	n := newTcpOutNode(t, map[string]any{
		"mode":            "reply",
		"appendDelimiter": "",
	}, reg)
	handle, peer := pairedSession(t, reg, "tcp-in-1")

	msg := flow.NewMessage()
	msg.Set("session", handle)
	msg.SetPayload("hello")

	go func() {
		_, _ = n.HandleMessage(msg)
	}()

	buf := make([]byte, 16)
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	got, err := peer.Read(buf)
	if err != nil {
		t.Fatalf("peer.Read: %v", err)
	}
	if string(buf[:got]) != "hello" {
		t.Errorf("got %q", buf[:got])
	}
}

func TestTcpOutReplyMissingSession(t *testing.T) {
	n := newTcpOutNode(t, map[string]any{"mode": "reply"}, nil)
	msg := flow.NewMessage()
	msg.SetPayload("x")
	_, err := n.HandleMessage(msg)
	if err == nil || !strings.Contains(err.Error(), "no session handle") {
		t.Errorf("got %v, want missing-session error", err)
	}
}

func TestTcpOutReplySessionClosed(t *testing.T) {
	reg := flow.NewSessionRegistry()
	n := newTcpOutNode(t, map[string]any{"mode": "reply"}, reg)
	handle, _ := pairedSession(t, reg, "tcp-in-1")
	reg.Close(handle.ID())

	msg := flow.NewMessage()
	msg.Set("session", handle)
	msg.SetPayload("x")
	_, err := n.HandleMessage(msg)
	if err == nil || !errors.Is(err, flow.ErrUnknownSession) {
		t.Errorf("got %v, want ErrUnknownSession", err)
	}
}

func TestTcpOutReplyAppendDelimiter(t *testing.T) {
	reg := flow.NewSessionRegistry()
	n := newTcpOutNode(t, map[string]any{
		"mode":            "reply",
		"appendDelimiter": `\r\n`,
	}, reg)
	handle, peer := pairedSession(t, reg, "tcp-in-1")

	go func() {
		msg := flow.NewMessage()
		msg.Set("session", handle)
		msg.SetPayload("PING")
		_, _ = n.HandleMessage(msg)
	}()

	buf := make([]byte, 16)
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	got, err := peer.Read(buf)
	if err != nil {
		t.Fatalf("peer.Read: %v", err)
	}
	if string(buf[:got]) != "PING\r\n" {
		t.Errorf("got %q, want PING\\r\\n", buf[:got])
	}
}

func TestTcpOutServerBroadcastFanOut(t *testing.T) {
	reg := flow.NewSessionRegistry()
	n := newTcpOutNode(t, map[string]any{
		"mode":         "server-broadcast",
		"targetTcpIn":  "tcp-in-target",
	}, reg)

	// Three simulated peers behind a single tcp-in.
	peers := make([]net.Conn, 3)
	for i := 0; i < 3; i++ {
		_, peers[i] = pairedSession(t, reg, "tcp-in-target")
	}
	// One peer behind a different tcp-in (must not receive).
	_, otherPeer := pairedSession(t, reg, "tcp-in-other")

	// Each peer reads concurrently — broadcast iterates sessions
	// in map order, so any sequential read on the test side could
	// block on whichever peer the broadcast happens to pick first.
	type res struct {
		i   int
		buf []byte
		err error
	}
	results := make(chan res, len(peers))
	for i, p := range peers {
		go func(i int, p net.Conn) {
			buf := make([]byte, 16)
			_ = p.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, err := p.Read(buf)
			results <- res{i: i, buf: buf[:n], err: err}
		}(i, p)
	}

	msg := flow.NewMessage()
	msg.SetPayload("BROADCAST")
	go func() {
		_, _ = n.HandleMessage(msg)
	}()

	for k := 0; k < len(peers); k++ {
		r := <-results
		if r.err != nil {
			t.Errorf("peer[%d] read: %v", r.i, r.err)
			continue
		}
		if string(r.buf) != "BROADCAST" {
			t.Errorf("peer[%d] got %q", r.i, r.buf)
		}
	}

	// Verify other-tcp-in peer received nothing.
	buf := make([]byte, 16)
	_ = otherPeer.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if got, _ := otherPeer.Read(buf); got > 0 {
		t.Errorf("other peer received %q (broadcast leaked)", buf[:got])
	}
}

func TestTcpOutServerBroadcastNoTargets(t *testing.T) {
	reg := flow.NewSessionRegistry()
	n := newTcpOutNode(t, map[string]any{
		"mode":        "server-broadcast",
		"targetTcpIn": "no-such-tcp-in",
	}, reg)
	msg := flow.NewMessage()
	msg.SetPayload("X")
	_, err := n.HandleMessage(msg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTcpOutClientPerMessageDial(t *testing.T) {
	l, port := listenLoopback(t)

	gotChan := make(chan []byte, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 64)
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		nb, _ := c.Read(buf)
		gotChan <- append([]byte{}, buf[:nb]...)
	}()

	n := newTcpOutNode(t, map[string]any{
		"mode":           "client",
		"host":           "127.0.0.1",
		"port":           port,
		"keepConnection": false,
	}, nil)
	msg := flow.NewMessage()
	msg.SetPayload("dial-and-send")
	if _, err := n.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := <-gotChan; string(got) != "dial-and-send" {
		t.Errorf("got %q", got)
	}
}

func TestTcpOutClientKeepConnectionReuses(t *testing.T) {
	l, port := listenLoopback(t)
	connCount := atomic.Int32{}
	gotChan := make(chan []byte, 4)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			connCount.Add(1)
			go func(conn net.Conn) {
				defer conn.Close()
				buf := make([]byte, 4096)
				_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
				for {
					nb, err := conn.Read(buf)
					if nb > 0 {
						gotChan <- append([]byte{}, buf[:nb]...)
					}
					if err != nil {
						return
					}
				}
			}(c)
		}
	}()

	n := newTcpOutNode(t, map[string]any{
		"mode":           "client",
		"host":           "127.0.0.1",
		"port":           port,
		"keepConnection": true,
	}, nil)

	for i := 0; i < 3; i++ {
		msg := flow.NewMessage()
		msg.SetPayload("msg" + strconv.Itoa(i))
		if _, err := n.HandleMessage(msg); err != nil {
			t.Fatalf("HandleMessage: %v", err)
		}
	}

	// TCP may coalesce multiple sends into one Read, so accumulate
	// bytes rather than count Read events.
	deadline := time.Now().Add(2 * time.Second)
	var got []byte
	for len(got) < len("msg0msg1msg2") && time.Now().Before(deadline) {
		select {
		case b := <-gotChan:
			got = append(got, b...)
		case <-time.After(50 * time.Millisecond):
		}
	}
	if string(got) != "msg0msg1msg2" {
		t.Fatalf("server received %q, want msg0msg1msg2", got)
	}
	if c := connCount.Load(); c != 1 {
		t.Errorf("server saw %d connections, want 1 (reuse)", c)
	}
}

func TestTcpOutClientQueueOverflow(t *testing.T) {
	// Don't start a server: dials will time out and the queue will
	// fill until overflow.
	port := pickPort(t)
	inst, err := NewTCPOutNode(flow.NodeConfig{
		ID: "tcp-out-overflow",
		Properties: map[string]any{
			"mode":              "client",
			"host":              "127.0.0.1",
			"port":              port,
			"keepConnection":    true,
			"outboundQueueSize": 2,
			"dialTimeout":       1, // 1 second; tests run fast enough
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	n := inst.(*TCPOutNode)
	if err := n.Init(); err != nil {
		t.Fatal(err)
	}
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(noopStatus)
	n.SetDebug(noopDebug)
	n.SetError(func(error, *flow.Message) {})
	n.SetSessionRegistry(flow.NewSessionRegistry())
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	// First two enqueues should succeed; subsequent ones should
	// overflow because the worker is parked on a slow dial.
	overflows := 0
	for i := 0; i < 6; i++ {
		msg := flow.NewMessage()
		msg.SetPayload("X")
		_, err := n.HandleMessage(msg)
		if err != nil && strings.Contains(err.Error(), "outbound queue full") {
			overflows++
		}
	}
	if overflows == 0 {
		t.Error("no queue overflow observed")
	}
}

func TestTcpOutClientEmptyHostError(t *testing.T) {
	n := newTcpOutNode(t, map[string]any{
		"mode":           "client",
		"keepConnection": false,
	}, nil)
	msg := flow.NewMessage()
	msg.SetPayload("x")
	_, err := n.HandleMessage(msg)
	if err == nil {
		t.Error("expected error for empty host")
	}
}

func TestTcpOutClientMsgHostPort(t *testing.T) {
	l, port := listenLoopback(t)
	gotChan := make(chan []byte, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 64)
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		nb, _ := c.Read(buf)
		gotChan <- append([]byte{}, buf[:nb]...)
	}()

	n := newTcpOutNode(t, map[string]any{
		"mode":           "client",
		"keepConnection": false,
	}, nil)
	msg := flow.NewMessage()
	msg.Set("host", "127.0.0.1")
	msg.Set("port", port)
	msg.SetPayload("via-msg")
	if _, err := n.HandleMessage(msg); err != nil {
		t.Fatal(err)
	}
	if got := <-gotChan; string(got) != "via-msg" {
		t.Errorf("got %q", got)
	}
}

func TestTcpOutInvalidMode(t *testing.T) {
	inst, err := NewTCPOutNode(flow.NodeConfig{
		ID: "x",
		Properties: map[string]any{"mode": "junk"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.Init(); err == nil {
		t.Error("expected Init error")
	}
}

func TestTcpOutBroadcastRequiresTarget(t *testing.T) {
	inst, err := NewTCPOutNode(flow.NodeConfig{
		ID: "x",
		Properties: map[string]any{"mode": "server-broadcast"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.Init(); err == nil {
		t.Error("expected Init error for missing targetTcpIn")
	}
}

// ─── End-to-end: tcp-in server + tcp-out reply ──────────────────────────────

func TestEndToEndTcpInToReplyOut(t *testing.T) {
	port := pickPort(t)
	reg := flow.NewSessionRegistry()

	// Stand up tcp-in (server mode).
	inInst, err := NewTCPInNode(flow.NodeConfig{
		ID:   "tcp-in-1",
		Type: "tcp-in",
		Properties: map[string]any{
			"mode":            "server",
			"host":            "127.0.0.1",
			"port":            port,
			"framing":         "delimiter",
			"delimiter":       `\n`,
			"payloadEncoding": "string",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	in := inInst.(*TCPInNode)
	if err := in.Init(); err != nil {
		t.Fatal(err)
	}

	// Plumb tcp-in's emitted messages straight into tcp-out reply.
	outInst, err := NewTCPOutNode(flow.NodeConfig{
		ID:         "tcp-out-1",
		Type:       "tcp-out",
		Properties: map[string]any{"mode": "reply", "appendDelimiter": `\n`},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := outInst.(*TCPOutNode)
	if err := out.Init(); err != nil {
		t.Fatal(err)
	}

	relay := func(_ int, m *flow.Message) {
		// Transform: uppercase the payload then reply.
		s, ok := m.Payload().(string)
		if !ok {
			return
		}
		m.SetPayload(strings.ToUpper(s))
		_, _ = out.HandleMessage(m)
	}

	in.SetSend(relay)
	in.SetStatus(noopStatus)
	in.SetDebug(noopDebug)
	in.SetError(func(error, *flow.Message) {})
	in.SetSessionRegistry(reg)
	if err := in.Start(); err != nil {
		t.Fatal(err)
	}
	defer in.Stop()

	out.SetSend(func(int, *flow.Message) {})
	out.SetStatus(noopStatus)
	out.SetDebug(noopDebug)
	out.SetError(func(error, *flow.Message) {})
	out.SetSessionRegistry(reg)
	if err := out.Start(); err != nil {
		t.Fatal(err)
	}
	defer out.Stop()

	// Connect a client and send a line.
	c := dialLoopback(t, port)
	if _, err := c.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}

	// Expect uppercase "HELLO\n" back.
	buf := make([]byte, 32)
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	nb, err := c.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("client read: %v", err)
	}
	if got := string(buf[:nb]); got != "HELLO\n" {
		t.Errorf("got %q, want HELLO\\n", got)
	}
}
