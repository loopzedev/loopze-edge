// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/niceclouds/flint/internal/flow"
)

func newOpcuaReadNodeForTest(t *testing.T, props map[string]any) *OpcuaReadNode {
	t.Helper()
	inst, err := NewOpcuaReadNode(flow.NodeConfig{
		ID: "read-1", Type: "opcua-read", FlowID: "flow-a",
		Properties: props,
	})
	if err != nil {
		t.Fatalf("NewOpcuaReadNode: %v", err)
	}
	return inst.(*OpcuaReadNode)
}

func TestOpcuaReadInitDefaults(t *testing.T) {
	n := newOpcuaReadNodeForTest(t, map[string]any{
		"server":  "srv-1",
		"nodeIds": []any{"i=2258"},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if n.mode != "triggered" {
		t.Errorf("default mode = %q; want triggered", n.mode)
	}
	if n.outputShape != "single" {
		t.Errorf("single nodeId default outputShape = %q; want single", n.outputShape)
	}
	if !n.includeMetadata {
		t.Error("includeMetadata default should be true")
	}
}

func TestOpcuaReadInitInvalidMode(t *testing.T) {
	n := newOpcuaReadNodeForTest(t, map[string]any{
		"server":  "srv-1",
		"nodeIds": []any{"i=2258"},
		"mode":    "bogus",
	})
	if err := n.Init(); err == nil {
		t.Error("expected error for invalid mode")
	}
}

func TestOpcuaReadInitMissingServer(t *testing.T) {
	n := newOpcuaReadNodeForTest(t, map[string]any{
		"nodeIds": []any{"i=2258"},
	})
	if err := n.Init(); err == nil {
		t.Error("expected error for missing server")
	}
}

func TestOpcuaReadInitMissingNodeIDs(t *testing.T) {
	n := newOpcuaReadNodeForTest(t, map[string]any{
		"server": "srv-1",
		"mode":   "static",
	})
	if err := n.Init(); err == nil {
		t.Error("static mode requires nodeIds")
	}

	// Dynamic mode allows empty config — control msg supplies them.
	n = newOpcuaReadNodeForTest(t, map[string]any{
		"server": "srv-1",
		"mode":   "dynamic",
	})
	if err := n.Init(); err != nil {
		t.Errorf("dynamic mode without nodeIds should be ok: %v", err)
	}
}

func TestOpcuaReadInitInvalidNodeID(t *testing.T) {
	n := newOpcuaReadNodeForTest(t, map[string]any{
		"server":  "srv-1",
		"nodeIds": []any{"this is not a nodeid"},
	})
	if err := n.Init(); err == nil {
		t.Error("expected error for unparseable nodeId")
	}
}

func TestExtractNodeIDs(t *testing.T) {
	cases := []struct {
		name string
		val  any
		want []string
	}{
		{"missing", nil, nil},
		{"empty string", "", nil},
		{"single string", "ns=2;i=1", []string{"ns=2;i=1"}},
		{"string slice", []string{"a", "b"}, []string{"a", "b"}},
		{"any slice", []any{"a", 7, "b", ""}, []string{"a", "b"}},
	}
	for _, c := range cases {
		msg := flow.NewMessage()
		if c.val != nil {
			msg.Set("nodeIds", c.val)
		}
		got := extractNodeIDs(msg)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v; want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v; want %v", c.name, got, c.want)
				break
			}
		}
	}
}

func TestParseNodeIDEntriesAcceptsLegacyAndNew(t *testing.T) {
	// Legacy: plain string array.
	got, err := parseNodeIDEntries([]any{"ns=2;i=1", "ns=2;i=2"})
	if err != nil {
		t.Fatalf("legacy: %v", err)
	}
	if len(got) != 2 || got[0].id != "ns=2;i=1" || got[0].name != "" {
		t.Errorf("legacy: %+v", got)
	}

	// New: object array with optional name.
	got, err = parseNodeIDEntries([]any{
		map[string]any{"id": "ns=2;i=1", "name": "Temp"},
		map[string]any{"id": "ns=2;i=2"},
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if got[0].name != "Temp" || got[1].name != "" {
		t.Errorf("new: %+v", got)
	}

	// Mixed should also work.
	got, err = parseNodeIDEntries([]any{
		"ns=2;i=1",
		map[string]any{"id": "ns=2;i=2", "name": "Pressure"},
	})
	if err != nil {
		t.Fatalf("mixed: %v", err)
	}
	if len(got) != 2 || got[1].name != "Pressure" {
		t.Errorf("mixed: %+v", got)
	}

	// Object form must accept legacy "nodeId" key too.
	got, err = parseNodeIDEntries([]any{
		map[string]any{"nodeId": "ns=2;i=3", "name": "Alt"},
	})
	if err != nil {
		t.Fatalf("nodeId-key form: %v", err)
	}
	if got[0].id != "ns=2;i=3" || got[0].name != "Alt" {
		t.Errorf("nodeId-key form: %+v", got)
	}
}

func TestShapePayloadByName(t *testing.T) {
	// User-supplied names take precedence.
	results := []map[string]any{
		{"nodeId": "ns=2;i=1", "name": "Temperature", "value": 23.5},
		{"nodeId": "ns=2;i=2", "name": "Pressure", "value": 1013.0},
	}
	n := &OpcuaReadNode{outputShape: "by-name", includeMetadata: false}
	obj := n.shapePayload(results).(map[string]any)
	if obj["Temperature"] != 23.5 || obj["Pressure"] != 1013.0 {
		t.Errorf("by-name simple: %+v", obj)
	}
}

func TestShapePayloadByNameDuplicateKeysGetSuffix(t *testing.T) {
	results := []map[string]any{
		{"nodeId": "ns=2;i=1", "name": "Pressure", "value": 1.0},
		{"nodeId": "ns=2;i=2", "name": "Pressure", "value": 2.0},
		{"nodeId": "ns=2;i=3", "name": "Pressure", "value": 3.0},
	}
	n := &OpcuaReadNode{outputShape: "by-name", includeMetadata: false}
	obj := n.shapePayload(results).(map[string]any)
	if obj["Pressure"] != 1.0 || obj["Pressure_2"] != 2.0 || obj["Pressure_3"] != 3.0 {
		t.Errorf("by-name dupe: %+v", obj)
	}
}

func TestShapePayloadByNameFallsBackToNodeID(t *testing.T) {
	results := []map[string]any{
		{"nodeId": "ns=2;i=1", "value": 23.5},
	}
	n := &OpcuaReadNode{outputShape: "by-name", includeMetadata: false}
	obj := n.shapePayload(results).(map[string]any)
	if obj["ns=2;i=1"] != 23.5 {
		t.Errorf("by-name fallback: %+v", obj)
	}
}

func TestShapePayloadForms(t *testing.T) {
	results := []map[string]any{
		{"nodeId": "ns=2;i=1", "value": 23.5, "statusCode": "Good", "statusCodeRaw": uint32(0)},
		{"nodeId": "ns=2;i=2", "value": int32(42), "statusCode": "Good", "statusCodeRaw": uint32(0)},
	}

	n := &OpcuaReadNode{outputShape: "array", includeMetadata: true}
	got := n.shapePayload(results).([]any)
	if len(got) != 2 {
		t.Fatalf("array len: %d", len(got))
	}

	n = &OpcuaReadNode{outputShape: "array", includeMetadata: false}
	got = n.shapePayload(results).([]any)
	if got[0] != 23.5 || got[1].(int32) != 42 {
		t.Errorf("flat array: %v", got)
	}

	n = &OpcuaReadNode{outputShape: "object", includeMetadata: false}
	obj := n.shapePayload(results).(map[string]any)
	if obj["ns=2;i=1"] != 23.5 {
		t.Errorf("object[a] = %v", obj["ns=2;i=1"])
	}

	n = &OpcuaReadNode{outputShape: "single", includeMetadata: true}
	single := n.shapePayload(results[:1]).(map[string]any)
	if single["nodeId"] != "ns=2;i=1" {
		t.Errorf("single nodeId: %v", single["nodeId"])
	}

	n = &OpcuaReadNode{outputShape: "single", includeMetadata: false}
	if v := n.shapePayload(results[:1]); v != 23.5 {
		t.Errorf("single value: %v", v)
	}
}

// E2E test against the Deno test server.
func TestOpcuaReadE2E(t *testing.T) {
	endpoint := opcuaTestEndpoint(t)

	server, err := NewOpcuaServer(flow.ConfigNode{
		ID: "srv-e2e", Type: "opcua-server",
		Config: map[string]any{"endpointUrl": endpoint},
	})
	if err != nil {
		t.Fatalf("NewOpcuaServer: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("server Start: %v", err)
	}
	defer server.Stop()

	// Wait for the connection.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fill, _ := server.Status(); fill == "green" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if fill, _ := server.Status(); fill != "green" {
		t.Fatal("server did not connect")
	}

	read, err := NewOpcuaReadNode(flow.NodeConfig{
		ID: "read-e2e", Type: "opcua-read", FlowID: "f",
		Properties: map[string]any{
			"server":          "srv-e2e",
			"mode":            "triggered",
			"nodeIds":         []any{"i=2258"}, // CurrentTime
			"outputShape":     "single",
			"includeMetadata": true,
		},
	})
	if err != nil {
		t.Fatalf("NewOpcuaReadNode: %v", err)
	}
	r := read.(*OpcuaReadNode)
	if err := r.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	r.SetSend(func(int, *flow.Message) {})
	r.SetStatus(func(string, string) {})
	r.SetConfigLookup(func(id string) (flow.ConfigInstance, bool) {
		if id == "srv-e2e" {
			return server, true
		}
		return nil, false
	})
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = ctx

	out, err := r.HandleMessage(flow.NewMessage())
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out) == 0 || len(out[0]) == 0 {
		t.Fatal("no output")
	}
	payload := out[0][0].Get("payload")
	m, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type: %T", payload)
	}
	if m["statusCode"] != "Good" {
		t.Errorf("statusCode = %v", m["statusCode"])
	}
	if m["value"] == nil {
		t.Error("value missing")
	}
	t.Logf("CurrentTime: %v (status=%v)", m["value"], m["statusCode"])
}

// Sanity: makes sure runStatic shuts down promptly when the node is stopped
// even if the read errors continuously (no client). We piggyback on the
// startupRead path — without an actual server, doRead returns nil and the
// goroutine waits on the ticker (or the stop channel).
func TestOpcuaReadStaticStops(t *testing.T) {
	n := newOpcuaReadNodeForTest(t, map[string]any{
		"server":      "srv-x",
		"mode":        "static",
		"nodeIds":     []any{"i=2258"},
		"interval":    50.0,
		"startupRead": false,
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Bypass Start() — we don't want a real server lookup. Manually run
	// the loop with a stub server reference set to nil; doRead handles that.
	stop := make(chan struct{})
	done := make(chan struct{})
	n.server = &OpcuaServer{requestTimeout: time.Second}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		n.runStatic(stop, done)
	}()
	time.Sleep(120 * time.Millisecond)
	close(stop)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runStatic did not exit after stop")
	}
	wg.Wait()
}
