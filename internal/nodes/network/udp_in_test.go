// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package network

import (
	"github.com/loopzedev/loopze-edge/internal/nodes/nodestest"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

type udpInHarness struct {
	t    *testing.T
	node *UDPInNode
	col  *nodestest.Collector
	errs atomic.Int32
}

func newUdpInHarness(t *testing.T, props map[string]any) *udpInHarness {
	t.Helper()
	inst, err := NewUDPInNode(flow.NodeConfig{
		ID:         "udp-in-1",
		Type:       "udp-in",
		Properties: props,
	})
	if err != nil {
		t.Fatal(err)
	}
	n := inst.(*UDPInNode)
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	col := &nodestest.Collector{}
	h := &udpInHarness{t: t, node: n, col: col}
	n.SetSend(col.Send)
	n.SetStatus(noopStatus)
	n.SetDebug(noopDebug)
	n.SetError(func(error, *flow.Message) { h.errs.Add(1) })
	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })
	return h
}

// pickUdpPort opens an ephemeral UDP socket, returns the port, closes
// the socket. Suitable when a test needs the port number BEFORE the
// node binds it.
func pickUdpPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	port := c.LocalAddr().(*net.UDPAddr).Port
	_ = c.Close()
	return port
}

func TestUdpInBasicReceive(t *testing.T) {
	port := pickUdpPort(t)
	h := newUdpInHarness(t, map[string]any{
		"host":            "127.0.0.1",
		"port":            port,
		"payloadEncoding": "string",
	})

	c, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.col.Count() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if h.col.Count() != 1 {
		t.Fatalf("got %d datagrams, want 1", h.col.Count())
	}
	got := h.col.At(0)
	if got.Payload() != "hello" {
		t.Errorf("payload = %v", got.Payload())
	}
	if ip, _ := got.Get("ip").(string); ip != "127.0.0.1" {
		t.Errorf("ip = %q", ip)
	}
	if size, _ := got.Get("size").(int); size != 5 {
		t.Errorf("size = %d", size)
	}
}

func TestUdpInBufferEncoding(t *testing.T) {
	port := pickUdpPort(t)
	h := newUdpInHarness(t, map[string]any{
		"host":            "127.0.0.1",
		"port":            port,
		"payloadEncoding": "buffer",
	})
	c, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	defer c.Close()
	_, _ = c.Write([]byte{0xDE, 0xAD, 0xBE, 0xEF})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && h.col.Count() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if h.col.Count() != 1 {
		t.Fatalf("got %d", h.col.Count())
	}
	arr, ok := h.col.At(0).Payload().([]int)
	if !ok {
		t.Fatalf("payload type %T", h.col.At(0).Payload())
	}
	want := []int{0xDE, 0xAD, 0xBE, 0xEF}
	if len(arr) != len(want) {
		t.Fatalf("len = %d", len(arr))
	}
	for i, v := range want {
		if arr[i] != v {
			t.Errorf("byte[%d] = %d, want %d", i, arr[i], v)
		}
	}
}

func TestUdpInBindError(t *testing.T) {
	// Bind a port, then try to bind a second udp-in to the same port.
	port := pickUdpPort(t)
	occupy, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer occupy.Close()

	inst, err := NewUDPInNode(flow.NodeConfig{
		ID: "udp-in-conflict",
		Properties: map[string]any{
			"host": "127.0.0.1",
			"port": port,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	n := inst.(*UDPInNode)
	if err := n.Init(); err != nil {
		t.Fatal(err)
	}
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(noopStatus)
	n.SetDebug(noopDebug)
	n.SetError(func(error, *flow.Message) {})
	if err := n.Start(); err == nil {
		t.Error("expected bind conflict error, got nil")
		_ = n.Stop()
	}
}

func TestUdpInAllowedRemotes(t *testing.T) {
	port := pickUdpPort(t)
	h := newUdpInHarness(t, map[string]any{
		"host":            "127.0.0.1",
		"port":            port,
		"payloadEncoding": "string",
		"allowedRemotes":  []any{"10.0.0.0/8"},
	})

	c, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	defer c.Close()
	_, _ = c.Write([]byte("blocked"))

	time.Sleep(150 * time.Millisecond)
	if h.col.Count() != 0 {
		t.Errorf("got %d messages from blocked remote", h.col.Count())
	}
}

func TestUdpInStopReleasesPort(t *testing.T) {
	port := pickUdpPort(t)
	h := newUdpInHarness(t, map[string]any{
		"host": "127.0.0.1",
		"port": port,
	})
	if err := h.node.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Port should be free again — confirm by binding ourselves.
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatalf("port not freed after Stop: %v", err)
	}
	_ = conn.Close()
}

func TestUdpInInvalidPayloadEncoding(t *testing.T) {
	inst, err := NewUDPInNode(flow.NodeConfig{
		ID: "x",
		Properties: map[string]any{
			"host":            "127.0.0.1",
			"port":            5000,
			"payloadEncoding": "junk",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.Init(); err == nil {
		t.Error("expected Init to reject junk encoding")
	}
}

func TestUdpInInvalidMulticastGroup(t *testing.T) {
	inst, err := NewUDPInNode(flow.NodeConfig{
		ID: "x",
		Properties: map[string]any{
			"host":            "0.0.0.0",
			"port":            5000,
			"multicastGroups": []any{"10.0.0.1"}, // not multicast
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.Init(); err == nil {
		t.Error("expected Init to reject non-multicast IP")
	}
}

// TestUdpInTypeInfo is a smoke check that the type metadata is sane.
func TestUdpInTypeInfo(t *testing.T) {
	info := UDPInTypeInfo()
	if info.Type != "udp-in" {
		t.Errorf("Type=%s", info.Type)
	}
	if info.Inputs != 0 || info.Outputs != 1 {
		t.Errorf("ports = %d→%d", info.Inputs, info.Outputs)
	}
	_ = strconv.Itoa(0) // silence import
}
