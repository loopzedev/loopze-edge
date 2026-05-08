// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

func newUdpOutNode(t *testing.T, props map[string]any) *UDPOutNode {
	t.Helper()
	inst, err := NewUDPOutNode(flow.NodeConfig{
		ID:         "udp-out-1",
		Type:       "udp-out",
		Properties: props,
	})
	if err != nil {
		t.Fatal(err)
	}
	n := inst.(*UDPOutNode)
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(noopStatus)
	n.SetDebug(noopDebug)
	n.SetError(func(error, *flow.Message) {})
	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })
	return n
}

// bindReceiver opens a UDP listener on the given port and returns the
// connection plus a port number. Test code is expected to send to that
// port AFTER this returns; readOne consumes one datagram with a
// timeout. Splitting bind and read avoids the race where the sender
// fires before the kernel registers the listener.
func bindReceiver(t *testing.T) (*net.UDPConn, int) {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	t.Cleanup(func() { _ = conn.Close() })
	return conn, port
}

func readOne(t *testing.T, conn *net.UDPConn, timeout time.Duration) ([]byte, string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 65536)
	nb, src, err := conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP: %v", err)
	}
	return append([]byte{}, buf[:nb]...), src.IP.String()
}

func TestUdpOutUnicast(t *testing.T) {
	conn, port := bindReceiver(t)

	n := newUdpOutNode(t, map[string]any{
		"host": "127.0.0.1",
		"port": port,
		"mode": "unicast",
	})
	msg := flow.NewMessage()
	msg.SetPayload("hello-udp")
	if _, err := n.HandleMessage(msg); err != nil {
		t.Fatal(err)
	}
	got, _ := readOne(t, conn, 2*time.Second)
	if string(got) != "hello-udp" {
		t.Errorf("got %q", got)
	}
}

func TestUdpOutMsgHostPortOverride(t *testing.T) {
	conn, port := bindReceiver(t)

	n := newUdpOutNode(t, map[string]any{
		"host": "127.0.0.99", // would not work
		"port": 1,
		"mode": "unicast",
	})
	msg := flow.NewMessage()
	msg.Set("host", "127.0.0.1")
	msg.Set("port", port)
	msg.SetPayload("via-msg")
	if _, err := n.HandleMessage(msg); err != nil {
		t.Fatal(err)
	}
	got, _ := readOne(t, conn, 2*time.Second)
	if string(got) != "via-msg" {
		t.Errorf("got %q", got)
	}
}

func TestUdpOutReuseSocket(t *testing.T) {
	conn, port := bindReceiver(t)

	n := newUdpOutNode(t, map[string]any{
		"host":        "127.0.0.1",
		"port":        port,
		"mode":        "unicast",
		"reuseSocket": true,
	})
	for i := 0; i < 3; i++ {
		msg := flow.NewMessage()
		msg.SetPayload("dgr" + strconv.Itoa(i))
		if _, err := n.HandleMessage(msg); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}

	var firstSrc string
	for i := 0; i < 3; i++ {
		body, src := readOne(t, conn, 2*time.Second)
		if i == 0 {
			firstSrc = src
		} else if src != firstSrc {
			t.Errorf("source addr changed: %s vs %s", src, firstSrc)
		}
		want := "dgr" + strconv.Itoa(i)
		if string(body) != want {
			t.Errorf("dgr[%d] = %q, want %q", i, body, want)
		}
	}
}

func TestUdpOutMulticastSetsTTL(t *testing.T) {
	// We can't easily verify the TTL on the wire from unit tests,
	// but we CAN verify Start succeeds with the multicast options
	// applied — i.e. SetMulticastTTL etc. don't error on the
	// loopback interface. Send a packet to a multicast group; the
	// loopback receive path picks it up when loopback=true.
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Skipf("multicast bind unavailable: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	_ = conn.Close()

	n := newUdpOutNode(t, map[string]any{
		"host":              "239.255.42.99",
		"port":              port,
		"mode":              "multicast",
		"multicastTTL":      1,
		"multicastLoopback": true,
	})
	msg := flow.NewMessage()
	msg.SetPayload("mcast")
	if _, err := n.HandleMessage(msg); err != nil {
		t.Fatalf("multicast send: %v", err)
	}
}

func TestUdpOutInvalidMode(t *testing.T) {
	inst, err := NewUDPOutNode(flow.NodeConfig{
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

func TestUdpOutEmptyHostError(t *testing.T) {
	n := newUdpOutNode(t, map[string]any{
		"mode": "unicast",
	})
	msg := flow.NewMessage()
	msg.SetPayload("x")
	_, err := n.HandleMessage(msg)
	if err == nil {
		t.Error("expected error for empty host")
	}
}

func TestUdpOutInvalidTTL(t *testing.T) {
	inst, err := NewUDPOutNode(flow.NodeConfig{
		ID: "x",
		Properties: map[string]any{
			"mode":         "multicast",
			"multicastTTL": 999,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.Init(); err == nil {
		t.Error("expected Init error for out-of-range TTL")
	}
}

func TestUdpOutPayloadByteArray(t *testing.T) {
	conn, port := bindReceiver(t)
	n := newUdpOutNode(t, map[string]any{
		"host": "127.0.0.1",
		"port": port,
		"mode": "unicast",
	})
	msg := flow.NewMessage()
	msg.SetPayload([]int{0xDE, 0xAD, 0xBE, 0xEF})
	if _, err := n.HandleMessage(msg); err != nil {
		t.Fatal(err)
	}
	got, _ := readOne(t, conn, 2*time.Second)
	want := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if string(got) != string(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// ─── End-to-end: udp-out → udp-in ────────────────────────────────────────────

func TestEndToEndUdpEcho(t *testing.T) {
	port := pickUdpPort(t)
	h := newUdpInHarness(t, map[string]any{
		"host":            "127.0.0.1",
		"port":            port,
		"payloadEncoding": "string",
	})

	out := newUdpOutNode(t, map[string]any{
		"host": "127.0.0.1",
		"port": port,
		"mode": "unicast",
	})
	msg := flow.NewMessage()
	msg.SetPayload("round-trip")
	if _, err := out.HandleMessage(msg); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && h.col.count() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if h.col.count() != 1 {
		t.Fatalf("got %d datagrams", h.col.count())
	}
	if got, _ := h.col.msgs[0].Payload().(string); got != "round-trip" {
		t.Errorf("payload = %q", got)
	}
}
