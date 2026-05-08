// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

func newTCPRequestNode(t *testing.T, props map[string]any) *TCPRequestNode {
	t.Helper()
	n, err := NewTCPRequestNode(flow.NodeConfig{
		ID:         "tcp-req-1",
		Type:       "tcp-request",
		Properties: props,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	tr := n.(*TCPRequestNode)
	tr.SetSend(func(int, *flow.Message) {})
	tr.SetStatus(noopStatus)
	tr.SetDebug(noopDebug)
	tr.SetError(func(error, *flow.Message) {})
	if err := tr.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = tr.Stop() })
	return tr
}

// servePayload starts a one-shot test server that immediately writes
// reply on the first accepted connection. When holdAfterReply is false
// it closes immediately (banner-style); otherwise it drains reads
// until the peer closes or a 5s deadline elapses, so the client can
// finish on its own deadline (used by the time-terminator test).
func servePayload(t *testing.T, l *net.TCPListener, reply []byte, holdAfterReply bool) {
	t.Helper()
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		if reply != nil {
			_, _ = c.Write(reply)
		}
		if !holdAfterReply {
			return
		}
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, _ = io.Copy(io.Discard, c)
	}()
}

func TestTcpRequestDelimiter(t *testing.T) {
	l, port := listenLoopback(t)
	servePayload(t, l, []byte("response-data\nGARBAGE-AFTER"), true)

	n := newTCPRequestNode(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             strconv.Itoa(port),
		"terminator":       "delimiter",
		"delimiter":        `\n`,
		"responseTimeout":  2000,
		"responseEncoding": "string",
	})
	msg := flow.NewMessage()
	msg.SetPayload("ping")
	out, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out) != 1 || len(out[0]) != 1 {
		t.Fatalf("expected 1 output message, got %v", out)
	}
	if got := out[0][0].Payload(); got != "response-data" {
		t.Errorf("payload = %q, want %q", got, "response-data")
	}
}

func TestTcpRequestLength(t *testing.T) {
	l, port := listenLoopback(t)
	servePayload(t, l, []byte("ABCDEFGH"), true)

	n := newTCPRequestNode(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             strconv.Itoa(port),
		"terminator":       "length",
		"responseLength":   4,
		"responseEncoding": "string",
		"responseTimeout":  2000,
	})
	out, err := n.HandleMessage(flow.NewMessage())
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Payload(); got != "ABCD" {
		t.Errorf("payload = %q, want ABCD", got)
	}
}

func TestTcpRequestLengthPrefix(t *testing.T) {
	l, port := listenLoopback(t)
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(5))
	buf.WriteString("hello")
	servePayload(t, l, buf.Bytes(), true)

	n := newTCPRequestNode(t, map[string]any{
		"host":       "127.0.0.1",
		"port":       strconv.Itoa(port),
		"terminator": "length-prefix",
		"lengthPrefix": map[string]any{
			"bytes":          4,
			"endianness":     "big",
			"includesHeader": false,
		},
		"responseEncoding": "string",
		"responseTimeout":  2000,
	})
	out, err := n.HandleMessage(flow.NewMessage())
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Payload(); got != "hello" {
		t.Errorf("payload = %q", got)
	}
}

func TestTcpRequestClose(t *testing.T) {
	l, port := listenLoopback(t)
	servePayload(t, l, []byte("banner-text-here"), false) // peer closes after reply

	n := newTCPRequestNode(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             strconv.Itoa(port),
		"terminator":       "close",
		"responseTimeout":  2000,
		"responseEncoding": "string",
	})
	out, err := n.HandleMessage(flow.NewMessage())
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Payload(); got != "banner-text-here" {
		t.Errorf("payload = %q", got)
	}
}

func TestTcpRequestTime(t *testing.T) {
	l, port := listenLoopback(t)
	servePayload(t, l, []byte("partial"), true) // hold conn open past timeout

	n := newTCPRequestNode(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             strconv.Itoa(port),
		"terminator":       "time",
		"responseTimeout":  150,
		"responseEncoding": "string",
	})
	out, err := n.HandleMessage(flow.NewMessage())
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Payload(); got != "partial" {
		t.Errorf("payload = %q, want partial", got)
	}
}

func TestTcpRequestTimeout(t *testing.T) {
	l, port := listenLoopback(t)
	// Accept but never reply, never close.
	go func() {
		c, _ := l.Accept()
		if c != nil {
			defer c.Close()
			time.Sleep(2 * time.Second)
		}
	}()

	n := newTCPRequestNode(t, map[string]any{
		"host":            "127.0.0.1",
		"port":            strconv.Itoa(port),
		"terminator":      "length",
		"responseLength":  10,
		"responseTimeout": 100,
	})
	_, err := n.HandleMessage(flow.NewMessage())
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestTcpRequestDialError(t *testing.T) {
	n := newTCPRequestNode(t, map[string]any{
		"host":            "127.0.0.1",
		"port":            "1", // privileged port; almost certainly closed
		"terminator":      "time",
		"responseTimeout": 100,
		"dialTimeout":     1,
	})
	_, err := n.HandleMessage(flow.NewMessage())
	if err == nil {
		t.Fatal("expected dial error")
	}
}

func TestTcpRequestMustacheHostPort(t *testing.T) {
	l, port := listenLoopback(t)
	servePayload(t, l, []byte("ok\n"), true)

	n := newTCPRequestNode(t, map[string]any{
		"host":             "{{target.host}}",
		"port":             "{{target.port}}",
		"terminator":       "delimiter",
		"delimiter":        `\n`,
		"responseEncoding": "string",
		"responseTimeout":  2000,
	})
	msg := flow.NewMessage()
	msg.Set("target", map[string]any{
		"host": "127.0.0.1",
		"port": strconv.Itoa(port),
	})
	out, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Payload(); got != "ok" {
		t.Errorf("payload = %q", got)
	}
}

func TestTcpRequestMsgHostPortOverride(t *testing.T) {
	l, port := listenLoopback(t)
	servePayload(t, l, []byte("ok\n"), true)

	n := newTCPRequestNode(t, map[string]any{
		"host":             "127.0.0.99", // would fail
		"port":             "1",
		"terminator":       "delimiter",
		"delimiter":        `\n`,
		"responseEncoding": "string",
		"responseTimeout":  2000,
	})
	msg := flow.NewMessage()
	msg.Set("host", "127.0.0.1")
	msg.Set("port", port)
	out, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Payload(); got != "ok" {
		t.Errorf("payload = %q", got)
	}
}

func TestTcpRequestAppendDelimiter(t *testing.T) {
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
		n, _ := c.Read(buf)
		gotChan <- append([]byte{}, buf[:n]...)
		_, _ = c.Write([]byte("ack\n"))
	}()

	n := newTCPRequestNode(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             strconv.Itoa(port),
		"terminator":       "delimiter",
		"delimiter":        `\n`,
		"appendDelimiter":  `\r\n`,
		"responseEncoding": "string",
		"responseTimeout":  2000,
	})
	msg := flow.NewMessage()
	msg.SetPayload("PING")
	if _, err := n.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := <-gotChan
	if !bytes.Equal(got, []byte("PING\r\n")) {
		t.Errorf("server received %q, want %q", got, "PING\r\n")
	}
}

func TestTcpRequestEmptyHostError(t *testing.T) {
	n := newTCPRequestNode(t, map[string]any{
		"host":            "",
		"port":            "1234",
		"terminator":      "time",
		"responseTimeout": 100,
	})
	if _, err := n.HandleMessage(flow.NewMessage()); err == nil {
		t.Error("expected error for empty host")
	}
}

func TestTcpRequestInitInvalidTerminator(t *testing.T) {
	n, err := NewTCPRequestNode(flow.NodeConfig{
		ID: "x",
		Properties: map[string]any{
			"host":       "1.2.3.4",
			"port":       "1234",
			"terminator": "junk",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Init(); err == nil {
		t.Error("expected Init to fail for junk terminator")
	}
}

func TestTcpRequestKeepConnectionRejectsCloseTerminator(t *testing.T) {
	n, err := NewTCPRequestNode(flow.NodeConfig{
		ID: "x",
		Properties: map[string]any{
			"host":           "1.2.3.4",
			"port":           "1234",
			"terminator":     "close",
			"keepConnection": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Init(); err == nil {
		t.Error("expected Init to reject close + keepConnection combination")
	}
}

func TestTcpRequestKeepConnectionReusesConn(t *testing.T) {
	l, port := listenLoopback(t)
	connCount := atomic.Int32{}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			connCount.Add(1)
			go func(conn net.Conn) {
				defer conn.Close()
				buf := make([]byte, 1024)
				_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				for {
					nb, err := conn.Read(buf)
					if nb > 0 {
						// Echo back the bytes followed by a newline so
						// the delimiter terminator sees the boundary.
						_, _ = conn.Write(buf[:nb])
					}
					if err != nil {
						return
					}
				}
			}(c)
		}
	}()

	n := newTCPRequestNode(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             strconv.Itoa(port),
		"terminator":       "delimiter",
		"delimiter":        `\n`,
		"appendDelimiter":  `\n`,
		"responseEncoding": "string",
		"responseTimeout":  2000,
		"keepConnection":   true,
	})

	for i := 0; i < 3; i++ {
		msg := flow.NewMessage()
		msg.SetPayload("hello" + strconv.Itoa(i))
		out, err := n.HandleMessage(msg)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		got, _ := out[0][0].Payload().(string)
		want := "hello" + strconv.Itoa(i)
		if got != want {
			t.Errorf("call %d payload = %q, want %q", i, got, want)
		}
		// First call dials; subsequent calls should report reuse.
		reused, _ := out[0][0].Get("connectionReused").(bool)
		if i == 0 && reused {
			t.Errorf("first call should not be a reuse")
		}
		if i > 0 && !reused {
			t.Errorf("call %d should report connectionReused=true", i)
		}
	}
	if c := connCount.Load(); c != 1 {
		t.Errorf("server saw %d accepts; want 1 (single persistent conn)", c)
	}
}

func TestTcpRequestKeepConnectionRedialsOnHostChange(t *testing.T) {
	// Two test servers on different ports; the same node calls both
	// with keepConnection=true. We expect two server accepts and
	// connectionReused=false on each call.
	l1, p1 := listenLoopback(t)
	l2, p2 := listenLoopback(t)

	startEcho := func(l *net.TCPListener) {
		go func() {
			c, err := l.Accept()
			if err != nil {
				return
			}
			defer c.Close()
			buf := make([]byte, 1024)
			_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
			for {
				nb, err := c.Read(buf)
				if nb > 0 {
					_, _ = c.Write(buf[:nb])
				}
				if err != nil {
					return
				}
			}
		}()
	}
	startEcho(l1)
	startEcho(l2)

	n := newTCPRequestNode(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             "0", // overridden via msg
		"terminator":       "delimiter",
		"delimiter":        `\n`,
		"appendDelimiter":  `\n`,
		"responseEncoding": "string",
		"responseTimeout":  2000,
		"keepConnection":   true,
	})

	for _, port := range []int{p1, p2} {
		msg := flow.NewMessage()
		msg.Set("port", port)
		msg.SetPayload("ping")
		out, err := n.HandleMessage(msg)
		if err != nil {
			t.Fatalf("port=%d: %v", port, err)
		}
		reused, _ := out[0][0].Get("connectionReused").(bool)
		if reused {
			t.Errorf("port=%d unexpectedly reported reuse", port)
		}
	}
}

func TestTcpRequestTLSHappyPath(t *testing.T) {
	certPEM, cert, _ := generateSelfSignedCert(t)
	l, port := listenLoopbackTLS(t, cert)

	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 64)
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _ = c.Read(buf)
		_, _ = c.Write([]byte("tls-ok\n"))
	}()

	n := newTCPRequestNode(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             strconv.Itoa(port),
		"terminator":       "delimiter",
		"delimiter":        `\n`,
		"appendDelimiter":  `\n`,
		"responseEncoding": "string",
		"responseTimeout":  2000,
		"tls": map[string]any{
			"enabled":    true,
			"serverName": "127.0.0.1",
			"caBundle":   string(certPEM),
		},
	})

	msg := flow.NewMessage()
	msg.SetPayload("ping")
	out, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("TLS round-trip: %v", err)
	}
	if got, _ := out[0][0].Payload().(string); got != "tls-ok" {
		t.Errorf("got %q", got)
	}
}

func TestTcpRequestTLSInsecureSkip(t *testing.T) {
	_, cert, _ := generateSelfSignedCert(t)
	l, port := listenLoopbackTLS(t, cert)

	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = c.Write([]byte("ok\n"))
	}()

	n := newTCPRequestNode(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             strconv.Itoa(port),
		"terminator":       "delimiter",
		"delimiter":        `\n`,
		"responseEncoding": "string",
		"responseTimeout":  2000,
		"tls": map[string]any{
			"enabled":            true,
			"insecureSkipVerify": true,
		},
	})

	msg := flow.NewMessage()
	if _, err := n.HandleMessage(msg); err != nil {
		t.Fatalf("expected success with insecureSkipVerify, got %v", err)
	}
}

func TestTcpRequestTLSVerifyFails(t *testing.T) {
	// Self-signed cert with no caBundle and no insecureSkipVerify
	// → handshake must fail because the system roots don't include
	// our test cert.
	_, cert, _ := generateSelfSignedCert(t)
	l, port := listenLoopbackTLS(t, cert)
	go func() {
		c, _ := l.Accept()
		if c != nil {
			defer c.Close()
		}
	}()

	n := newTCPRequestNode(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             strconv.Itoa(port),
		"terminator":       "delimiter",
		"delimiter":        `\n`,
		"responseEncoding": "string",
		"responseTimeout":  2000,
		"tls": map[string]any{
			"enabled":    true,
			"serverName": "127.0.0.1",
		},
	})

	msg := flow.NewMessage()
	if _, err := n.HandleMessage(msg); err == nil {
		t.Error("expected TLS verification failure")
	}
}

func TestTcpRequestTLSInvalidCABundle(t *testing.T) {
	inst, err := NewTCPRequestNode(flow.NodeConfig{
		ID: "x",
		Properties: map[string]any{
			"host": "1.2.3.4",
			"port": "1234",
			"tls": map[string]any{
				"enabled":  true,
				"caBundle": "not a real PEM bundle",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.Init(); err == nil {
		t.Error("expected Init to reject malformed PEM")
	}
}

func TestTcpRequestKeepConnectionRecoversAfterPeerClose(t *testing.T) {
	l, port := listenLoopback(t)
	// First conn: accept, echo one line, then close.
	// Second conn: accept, echo, hold open.
	go func() {
		first, _ := l.Accept()
		if first != nil {
			buf := make([]byte, 64)
			_ = first.SetReadDeadline(time.Now().Add(time.Second))
			nb, _ := first.Read(buf)
			if nb > 0 {
				_, _ = first.Write(buf[:nb])
			}
			_ = first.Close()
		}
		second, _ := l.Accept()
		if second != nil {
			defer second.Close()
			buf := make([]byte, 64)
			_ = second.SetReadDeadline(time.Now().Add(2 * time.Second))
			nb, _ := second.Read(buf)
			if nb > 0 {
				_, _ = second.Write(buf[:nb])
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	n := newTCPRequestNode(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             strconv.Itoa(port),
		"terminator":       "delimiter",
		"delimiter":        `\n`,
		"appendDelimiter":  `\n`,
		"responseEncoding": "string",
		"responseTimeout":  2000,
		"keepConnection":   true,
	})

	// Call #1 — succeeds, peer then closes.
	msg := flow.NewMessage()
	msg.SetPayload("first")
	if _, err := n.HandleMessage(msg); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Give the server-side close a moment to propagate.
	time.Sleep(50 * time.Millisecond)

	// Call #2 — the cached conn is dead. The implementation should
	// observe the broken pipe, redial against the freshly-accepted
	// server, and return successfully.
	msg2 := flow.NewMessage()
	msg2.SetPayload("second")
	out, err := n.HandleMessage(msg2)
	if err != nil {
		// Permit one transient failure and try once more — some
		// kernels surface the FIN only on the next write, in which
		// case the impl tears down the conn and the third call
		// would succeed. For a unit test we keep it simple: the
		// implementation must redial automatically.
		t.Fatalf("second call: %v", err)
	}
	if got, _ := out[0][0].Payload().(string); got != "second" {
		t.Errorf("second payload = %q", got)
	}
}
