// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// tcpInHarness wires a TCPInNode for tests, capturing messages, errors,
// and status updates.
type tcpInHarness struct {
	t        *testing.T
	node     *TCPInNode
	col      *collector
	registry *flow.SessionRegistry
	errs     atomic.Int32
}

func newTcpInHarness(t *testing.T, props map[string]any) *tcpInHarness {
	t.Helper()
	inst, err := NewTCPInNode(flow.NodeConfig{
		ID:         "tcp-in-1",
		Type:       "tcp-in",
		Properties: props,
	})
	if err != nil {
		t.Fatal(err)
	}
	n := inst.(*TCPInNode)
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	col := &collector{}
	reg := flow.NewSessionRegistry()
	h := &tcpInHarness{t: t, node: n, col: col, registry: reg}
	n.SetSend(col.send)
	n.SetStatus(noopStatus)
	n.SetDebug(noopDebug)
	n.SetError(func(error, *flow.Message) { h.errs.Add(1) })
	n.SetSessionRegistry(reg)
	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })
	return h
}

// pickPort opens a temporary listener, returns the port, and closes
// the listener. Suitable when the test wants the port number BEFORE it
// boots the actual server (e.g. before tcp-in starts).
func pickPort(t *testing.T) int {
	t.Helper()
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// waitForCount polls col.count() until it reaches want or the deadline
// elapses. Reports the actual count if the wait fails.
func waitForCount(t *testing.T, col *collector, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if col.count() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("waitForCount: got %d, want %d", col.count(), want)
}

func TestTcpInServerAcceptAndFrameDelimiter(t *testing.T) {
	port := pickPort(t)
	h := newTcpInHarness(t, map[string]any{
		"mode":             "server",
		"host":             "127.0.0.1",
		"port":             port,
		"framing":          "delimiter",
		"delimiter":        `\n`,
		"payloadEncoding":  "string",
	})

	c := dialLoopback(t, port)
	if _, err := c.Write([]byte("hello\nworld\n")); err != nil {
		t.Fatal(err)
	}
	waitForCount(t, h.col, 2, time.Second)

	if got := h.col.msgs[0].Payload(); got != "hello" {
		t.Errorf("frame[0] = %q", got)
	}
	if got := h.col.msgs[1].Payload(); got != "world" {
		t.Errorf("frame[1] = %q", got)
	}
	// session handle present and resolvable.
	handle, _ := h.col.msgs[0].Get("session").(*flow.SessionHandle)
	if handle == nil {
		t.Fatal("missing session handle on emitted message")
	}
	if _, err := h.registry.Resolve(handle.ID()); err != nil {
		t.Errorf("Resolve: %v", err)
	}
}

func TestTcpInServerLengthPrefix(t *testing.T) {
	port := pickPort(t)
	h := newTcpInHarness(t, map[string]any{
		"mode":            "server",
		"host":            "127.0.0.1",
		"port":            port,
		"framing":         "length-prefix",
		"lengthPrefix": map[string]any{
			"bytes":          4,
			"endianness":     "big",
			"includesHeader": false,
		},
		"payloadEncoding": "string",
	})

	c := dialLoopback(t, port)
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(5))
	buf.WriteString("hello")
	_, _ = c.Write(buf.Bytes())
	waitForCount(t, h.col, 1, time.Second)

	if got := h.col.msgs[0].Payload(); got != "hello" {
		t.Errorf("payload = %q", got)
	}
}

func TestTcpInServerFixedLength(t *testing.T) {
	port := pickPort(t)
	h := newTcpInHarness(t, map[string]any{
		"mode":            "server",
		"host":            "127.0.0.1",
		"port":            port,
		"framing":         "fixed-length",
		"fixedLength":     4,
		"payloadEncoding": "string",
	})
	c := dialLoopback(t, port)
	_, _ = c.Write([]byte("aaaabbbb"))
	waitForCount(t, h.col, 2, time.Second)
	if got := h.col.msgs[0].Payload(); got != "aaaa" {
		t.Errorf("frame[0] = %q", got)
	}
	if got := h.col.msgs[1].Payload(); got != "bbbb" {
		t.Errorf("frame[1] = %q", got)
	}
}

func TestTcpInServerOversizeFrame(t *testing.T) {
	port := pickPort(t)
	h := newTcpInHarness(t, map[string]any{
		"mode":            "server",
		"host":            "127.0.0.1",
		"port":            port,
		"framing":         "delimiter",
		"delimiter":       `\n`,
		"maxFrameBytes":   8,
		"payloadEncoding": "string",
	})
	c := dialLoopback(t, port)
	_, _ = c.Write([]byte("toolongtoolongtoolong"))
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.errs.Load() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected oversize error; errs=%d", h.errs.Load())
}

func TestTcpInServerPeerDisconnect(t *testing.T) {
	port := pickPort(t)
	h := newTcpInHarness(t, map[string]any{
		"mode":            "server",
		"host":            "127.0.0.1",
		"port":            port,
		"framing":         "delimiter",
		"delimiter":       `\n`,
		"payloadEncoding": "string",
		"emitCloseEvent":  true,
	})
	c := dialLoopback(t, port)
	_, _ = c.Write([]byte("hi\n"))
	waitForCount(t, h.col, 1, time.Second)
	_ = c.Close()
	// Wait for the close marker.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		last := h.col.last()
		if last != nil && last.Get("_event") == "close" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("close marker not emitted")
}

func TestTcpInServerCloseEventDisabledByDefault(t *testing.T) {
	port := pickPort(t)
	h := newTcpInHarness(t, map[string]any{
		"mode":            "server",
		"host":            "127.0.0.1",
		"port":            port,
		"framing":         "delimiter",
		"delimiter":       `\n`,
		"payloadEncoding": "string",
	})
	c := dialLoopback(t, port)
	_, _ = c.Write([]byte("hi\n"))
	waitForCount(t, h.col, 1, time.Second)
	_ = c.Close()
	// Wait briefly to give the server a chance to (incorrectly) emit
	// a close marker; expect that no second message appears.
	time.Sleep(150 * time.Millisecond)
	if h.col.count() != 1 {
		t.Errorf("got %d messages; want 1 (close event should be suppressed by default)", h.col.count())
	}
}

func TestTcpInServerAllowedRemotes(t *testing.T) {
	port := pickPort(t)
	h := newTcpInHarness(t, map[string]any{
		"mode":             "server",
		"host":             "127.0.0.1",
		"port":             port,
		"framing":          "delimiter",
		"delimiter":        `\n`,
		"payloadEncoding":  "string",
		"allowedRemotes":   []any{"10.0.0.0/8"},
	})
	c := dialLoopback(t, port)
	_, _ = c.Write([]byte("hi\n"))
	// We expect the conn to be closed by the server quickly. Verify
	// no message arrived within the wait window.
	time.Sleep(100 * time.Millisecond)
	if h.col.count() != 0 {
		t.Errorf("got %d messages from blocked remote", h.col.count())
	}
}

func TestTcpInServerStopClosesAllSessions(t *testing.T) {
	port := pickPort(t)
	h := newTcpInHarness(t, map[string]any{
		"mode":            "server",
		"host":            "127.0.0.1",
		"port":            port,
		"framing":         "delimiter",
		"delimiter":       `\n`,
		"payloadEncoding": "string",
	})
	c1 := dialLoopback(t, port)
	c2 := dialLoopback(t, port)
	_, _ = c1.Write([]byte("a\n"))
	_, _ = c2.Write([]byte("b\n"))
	waitForCount(t, h.col, 2, time.Second)
	if got := h.registry.LenByOwner("tcp-in-1"); got != 2 {
		t.Errorf("LenByOwner before Stop = %d", got)
	}

	if err := h.node.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := h.registry.LenByOwner("tcp-in-1"); got != 0 {
		t.Errorf("LenByOwner after Stop = %d", got)
	}
}

func TestTcpInServerInvalidPort(t *testing.T) {
	inst, err := NewTCPInNode(flow.NodeConfig{
		ID: "x",
		Properties: map[string]any{
			"mode": "server",
			"host": "127.0.0.1",
			"port": 0,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.Init(); err == nil {
		t.Error("expected Init error for port 0")
	}
}

func TestTcpInClientConnects(t *testing.T) {
	// Stand up a remote server that the client tcp-in dials into.
	l, port := listenLoopback(t)
	servePayload(t, l, []byte("greetings\n"), true)

	h := newTcpInHarness(t, map[string]any{
		"mode":              "client",
		"host":              "127.0.0.1",
		"port":              port,
		"framing":           "delimiter",
		"delimiter":         `\n`,
		"payloadEncoding":   "string",
		"reconnect":         false,
		"reconnectInitialDelay": 50,
	})
	waitForCount(t, h.col, 1, 2*time.Second)
	if got := h.col.msgs[0].Payload(); got != "greetings" {
		t.Errorf("payload = %q", got)
	}
}

func TestTcpInClientNoReconnectOnDialError(t *testing.T) {
	port := pickPort(t) // unbound
	inst, err := NewTCPInNode(flow.NodeConfig{
		ID: "client-noreconnect",
		Properties: map[string]any{
			"mode":              "client",
			"host":              "127.0.0.1",
			"port":              port,
			"reconnect":         false,
			"dialTimeout":       1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	n := inst.(*TCPInNode)
	if err := n.Init(); err != nil {
		t.Fatal(err)
	}
	col := &collector{}
	var errCount atomic.Int32
	n.SetSend(col.send)
	n.SetStatus(noopStatus)
	n.SetDebug(noopDebug)
	n.SetError(func(error, *flow.Message) { errCount.Add(1) })
	n.SetSessionRegistry(flow.NewSessionRegistry())
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	// Wait long enough for the dial to fail.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if errCount.Load() > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("no dial error reported; errs=%d", errCount.Load())
}

func TestTcpInServerStreamFraming(t *testing.T) {
	port := pickPort(t)
	h := newTcpInHarness(t, map[string]any{
		"mode":            "server",
		"host":            "127.0.0.1",
		"port":            port,
		"framing":         "stream",
		"payloadEncoding": "string",
	})
	c := dialLoopback(t, port)
	_, _ = c.Write([]byte("chunk1"))
	waitForCount(t, h.col, 1, time.Second)
	if got := h.col.msgs[0].Payload(); got != "chunk1" {
		t.Errorf("payload = %q", got)
	}
}

func TestTcpInClientReconnectAfterServerRestart(t *testing.T) {
	// Pick a port, start a server briefly, let the client connect,
	// stop the server, restart it, verify the client reconnects and
	// receives a second frame.
	port := pickPort(t)
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}

	l1, err := net.ListenTCP("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		c, err := l1.Accept()
		if err == nil {
			_, _ = c.Write([]byte("first\n"))
			time.Sleep(50 * time.Millisecond)
			_ = c.Close()
		}
	}()

	h := newTcpInHarness(t, map[string]any{
		"mode":                  "client",
		"host":                  "127.0.0.1",
		"port":                  port,
		"framing":               "delimiter",
		"delimiter":             `\n`,
		"payloadEncoding":       "string",
		"reconnect":             true,
		"reconnectInitialDelay": 50,
		"reconnectMaxDelay":     200,
		"dialTimeout":           1,
	})

	waitForCount(t, h.col, 1, 2*time.Second)
	if got, _ := h.col.snapshot()[0].Payload().(string); got != "first" {
		t.Fatalf("first frame = %q", got)
	}

	_ = l1.Close()
	// brief wait to let backoff kick in
	time.Sleep(100 * time.Millisecond)

	l2, err := net.ListenTCP("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	go func() {
		c, err := l2.Accept()
		if err == nil {
			_, _ = c.Write([]byte("second\n"))
			time.Sleep(50 * time.Millisecond)
			_ = c.Close()
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		// look for a "second" frame in any msg — the producer may
		// still be active so iterate a snapshot rather than the live
		// slice (race detector would otherwise flag the unsynchronised
		// read against the collector's append).
		for _, m := range h.col.snapshot() {
			if s, _ := m.Payload().(string); s == "second" {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("never received reconnected frame; got %d msgs total", h.col.count())
}

// ─── Type info smoke ────────────────────────────────────────────────────────

func TestTCPInTypeInfo(t *testing.T) {
	info := TCPInTypeInfo()
	if info.Type != "tcp-in" {
		t.Errorf("Type=%s", info.Type)
	}
	if info.Inputs != 0 || info.Outputs != 1 {
		t.Errorf("ports = %d→%d", info.Inputs, info.Outputs)
	}
	if _, ok := info.Defaults["mode"]; !ok {
		t.Error("missing mode default")
	}
	_ = strconv.Itoa(0) // silence unused import in some builds
}
