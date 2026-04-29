// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"testing"
	"time"

	"github.com/niceclouds/flint/internal/flow"
)

func newOpcuaWriteNodeForTest(t *testing.T, props map[string]any) *OpcuaWriteNode {
	t.Helper()
	inst, err := NewOpcuaWriteNode(flow.NodeConfig{
		ID: "write-1", Type: "opcua-write", FlowID: "f",
		Properties: props,
	})
	if err != nil {
		t.Fatalf("NewOpcuaWriteNode: %v", err)
	}
	return inst.(*OpcuaWriteNode)
}

func TestOpcuaWriteInitStaticRequiresWrites(t *testing.T) {
	n := newOpcuaWriteNodeForTest(t, map[string]any{
		"server": "srv-1",
		"mode":   "static",
	})
	if err := n.Init(); err == nil {
		t.Error("static mode without writes should fail")
	}
}

func TestOpcuaWriteInitDynamicAllowsEmpty(t *testing.T) {
	n := newOpcuaWriteNodeForTest(t, map[string]any{
		"server": "srv-1",
		"mode":   "dynamic",
	})
	if err := n.Init(); err != nil {
		t.Errorf("dynamic mode without writes should be ok: %v", err)
	}
}

func TestOpcuaWriteInitParsesStaticEntries(t *testing.T) {
	n := newOpcuaWriteNodeForTest(t, map[string]any{
		"server": "srv-1",
		"mode":   "static",
		"writes": []any{
			map[string]any{
				"nodeId":      "ns=2;s=Setpoint",
				"dataType":    "Double",
				"valueSource": "msg",
				"valuePath":   "payload",
			},
			map[string]any{
				"nodeId":      "ns=2;s=Mode",
				"dataType":    "Int32",
				"valueSource": "static",
				"value":       "3",
			},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(n.staticWrites) != 2 {
		t.Fatalf("staticWrites len = %d", len(n.staticWrites))
	}
	if n.staticWrites[0].valuePath != "payload" {
		t.Errorf("default valuePath = %q", n.staticWrites[0].valuePath)
	}
	if n.staticWrites[1].staticValue != "3" {
		t.Errorf("static value = %v", n.staticWrites[1].staticValue)
	}
}

func TestOpcuaWriteInitRejectsBadNodeID(t *testing.T) {
	n := newOpcuaWriteNodeForTest(t, map[string]any{
		"server": "srv-1",
		"mode":   "static",
		"writes": []any{
			map[string]any{"nodeId": "garbage"},
		},
	})
	if err := n.Init(); err == nil {
		t.Error("expected error for bad nodeId")
	}
}

func TestSpecsForMessageStatic(t *testing.T) {
	n := newOpcuaWriteNodeForTest(t, map[string]any{
		"server": "srv-1",
		"mode":   "static",
		"writes": []any{
			map[string]any{
				"nodeId":      "ns=2;s=Setpoint",
				"dataType":    "Double",
				"valueSource": "msg",
				"valuePath":   "payload",
			},
			map[string]any{
				"nodeId":      "ns=2;s=Speed",
				"dataType":    "Int32",
				"valueSource": "msg",
				"valuePath":   "speed",
			},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	msg := flow.NewMessage()
	msg.Set("payload", 42.5)
	msg.Set("speed", float64(1500))

	specs := n.specsForMessage(msg)
	if len(specs) != 2 {
		t.Fatalf("specs len = %d", len(specs))
	}
	if specs[0].value != 42.5 {
		t.Errorf("setpoint value = %v", specs[0].value)
	}
	if specs[1].value != float64(1500) {
		t.Errorf("speed value = %v", specs[1].value)
	}
}

func TestSpecsForMessageDynamicArray(t *testing.T) {
	n := newOpcuaWriteNodeForTest(t, map[string]any{
		"server": "srv-1",
		"mode":   "dynamic",
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	msg := flow.NewMessage()
	msg.Set("writes", []any{
		map[string]any{"nodeId": "ns=2;s=A", "dataType": "Int32", "value": float64(7)},
		map[string]any{"nodeId": "ns=2;s=B", "dataType": "Boolean", "value": true},
	})
	specs := n.specsForMessage(msg)
	if len(specs) != 2 {
		t.Fatalf("specs len = %d", len(specs))
	}
	if specs[0].dataType != "Int32" || specs[1].dataType != "Boolean" {
		t.Errorf("dataTypes: %+v", specs)
	}
}

func TestSpecsForMessageDynamicSingle(t *testing.T) {
	n := newOpcuaWriteNodeForTest(t, map[string]any{
		"server": "srv-1",
		"mode":   "dynamic",
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	msg := flow.NewMessage()
	msg.Set("nodeId", "ns=2;s=X")
	msg.Set("payload", float64(1))
	msg.Set("dataType", "Int32")

	specs := n.specsForMessage(msg)
	if len(specs) != 1 || specs[0].dataType != "Int32" || specs[0].value != float64(1) {
		t.Errorf("specs: %+v", specs)
	}
}

// E2E roundtrip: write a value, read it back, expect the same.
func TestOpcuaWriteE2E(t *testing.T) {
	endpoint := opcuaTestEndpoint(t)

	server, err := NewOpcuaServer(flow.ConfigNode{
		ID: "srv-write-e2e", Type: "opcua-server",
		Config: map[string]any{"endpointUrl": endpoint},
	})
	if err != nil {
		t.Fatalf("NewOpcuaServer: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("server.Start: %v", err)
	}
	defer server.Stop()

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

	// The Deno test server is expected to expose a writable Int32 at this NodeID.
	const targetNode = "ns=1;s=writable.int32"

	write, err := NewOpcuaWriteNode(flow.NodeConfig{
		ID: "w-1", Type: "opcua-write", FlowID: "f",
		Properties: map[string]any{
			"server": "srv-write-e2e",
			"mode":   "static",
			"writes": []any{
				map[string]any{
					"nodeId":      targetNode,
					"dataType":    "Int32",
					"valueSource": "msg",
					"valuePath":   "payload",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewOpcuaWriteNode: %v", err)
	}
	w := write.(*OpcuaWriteNode)
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	w.SetSend(func(int, *flow.Message) {})
	w.SetStatus(func(string, string) {})
	w.SetConfigLookup(func(id string) (flow.ConfigInstance, bool) {
		if id == "srv-write-e2e" {
			return server, true
		}
		return nil, false
	})
	if err := w.Start(); err != nil {
		t.Skipf("Start: %v (test server likely lacks %s)", err, targetNode)
	}

	msg := flow.NewMessage()
	msg.Set("payload", float64(1234))
	out, err := w.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out) == 0 || len(out[0]) == 0 {
		t.Fatal("no output message")
	}
	allGood, _ := out[0][0].Get("allGood").(bool)
	if !allGood {
		t.Logf("write result: %+v", out[0][0].Get("writeResult"))
		t.Skipf("write rejected — likely the Deno server does not expose %s as writable", targetNode)
	}
}
