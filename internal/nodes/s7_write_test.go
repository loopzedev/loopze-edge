// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"strings"
	"sync"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// ─── Unit tests (no PLC required) ─────────────────────────────────────────

func TestS7Write_InitRejectsMissingPLC(t *testing.T) {
	n := newS7WriteNode(t, map[string]any{})
	if err := n.Init(); err == nil {
		t.Fatal("Init should reject missing plc property")
	}
}

func TestS7Write_InitRejectsInvalidMode(t *testing.T) {
	n := newS7WriteNode(t, map[string]any{"plc": "p1", "mode": "fancy"})
	if err := n.Init(); err == nil {
		t.Fatal("Init should reject invalid mode")
	}
}

func TestS7Write_InitBlockMode_RequiresBlockConfig(t *testing.T) {
	n := newS7WriteNode(t, map[string]any{"plc": "p1", "mode": "block"})
	if err := n.Init(); err == nil {
		t.Fatal("Init should reject block mode without `block` config")
	}
}

func TestS7Write_InitBlockMode_RejectsInputArea(t *testing.T) {
	n := newS7WriteNode(t, map[string]any{
		"plc": "p1", "mode": "block",
		"block": map[string]any{"area": "I", "start": 0},
	})
	if err := n.Init(); err == nil || !strings.Contains(err.Error(), "not writable") {
		t.Errorf("expected I-area rejection, got %v", err)
	}
}

func TestS7Write_InitBlockMode_DBRequiresDBNumber(t *testing.T) {
	n := newS7WriteNode(t, map[string]any{
		"plc": "p1", "mode": "block",
		"block": map[string]any{"area": "DB", "start": 0},
	})
	if err := n.Init(); err == nil {
		t.Fatal("Init should reject block.area=DB without db>0")
	}
}

func TestS7Write_InitBlockMode_DefaultInputProperty(t *testing.T) {
	n := newS7WriteNode(t, map[string]any{
		"plc": "p1", "mode": "block",
		"block": map[string]any{"area": "M", "start": 0},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if n.block.InputProperty != "payload" {
		t.Errorf("InputProperty default: got %q, want payload", n.block.InputProperty)
	}
}

func TestS7Write_InitRejectsStaticWithNoVariables(t *testing.T) {
	n := newS7WriteNode(t, map[string]any{"plc": "p1", "mode": "static"})
	if err := n.Init(); err == nil {
		t.Fatal("Init should reject static mode with empty variables")
	}
}

func TestS7Write_InitAcceptsDynamicWithNoVariables(t *testing.T) {
	n := newS7WriteNode(t, map[string]any{"plc": "p1", "mode": "dynamic"})
	if err := n.Init(); err != nil {
		t.Fatalf("Init should accept dynamic mode with empty variables: %v", err)
	}
}

func TestS7Write_InitRejectsAckAndPassthroughTogether(t *testing.T) {
	n := newS7WriteNode(t, map[string]any{
		"plc": "p1", "mode": "dynamic",
		"emitAck": true, "passthrough": true,
	})
	if err := n.Init(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutually-exclusive error, got %v", err)
	}
}

func TestS7Write_InitRejectsStaticValueSourceMissingValue(t *testing.T) {
	n := newS7WriteNode(t, map[string]any{
		"plc": "p1", "mode": "static",
		"variables": []any{
			map[string]any{
				"address":     "MD0",
				"dataType":    "dword",
				"valueSource": "static",
				// no `value`
			},
		},
	})
	if err := n.Init(); err == nil || !strings.Contains(err.Error(), "valueSource=static requires") {
		t.Errorf("expected static-value error, got %v", err)
	}
}

func TestS7Write_InitParsesStaticVariables(t *testing.T) {
	n := newS7WriteNode(t, map[string]any{
		"plc": "p1", "mode": "static",
		"variables": []any{
			map[string]any{
				"address": "DB10.DBD0", "dataType": "real",
				"valueSource": "msg", "valuePath": "payload.setpoint",
			},
			map[string]any{
				"address": "M0.0", "dataType": "bool",
				"valueSource": "static", "value": true,
			},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(n.variables) != 2 {
		t.Fatalf("variables: got %d, want 2", len(n.variables))
	}
	if n.variables[0].ValuePath != "payload.setpoint" {
		t.Errorf("v0.ValuePath: got %q, want payload.setpoint", n.variables[0].ValuePath)
	}
	if !n.variables[1].HasStatic || n.variables[1].StaticValue != true {
		t.Errorf("v1: expected HasStatic=true, StaticValue=true; got %+v", n.variables[1])
	}
}

func TestS7Write_InitRejectsBadAddress(t *testing.T) {
	n := newS7WriteNode(t, map[string]any{
		"plc": "p1", "mode": "static",
		"variables": []any{
			map[string]any{"address": "DB1.MotorSpeed", "dataType": "real", "valueSource": "msg"},
		},
	})
	if err := n.Init(); err == nil || !strings.Contains(err.Error(), "use OPC UA") {
		t.Errorf("expected OPC UA hint for symbolic address, got %v", err)
	}
}

func TestS7Write_InitRejectsInvalidValueSource(t *testing.T) {
	n := newS7WriteNode(t, map[string]any{
		"plc": "p1", "mode": "static",
		"variables": []any{
			map[string]any{"address": "MD0", "dataType": "dword", "valueSource": "weird"},
		},
	})
	if err := n.Init(); err == nil {
		t.Fatal("Init should reject invalid valueSource")
	}
}

func TestS7WriteTypeInfo(t *testing.T) {
	info := S7WriteTypeInfo()
	if info.Type != "s7-write" {
		t.Errorf("Type: got %q, want s7-write", info.Type)
	}
	if info.Inputs != 1 || info.Outputs != 0 {
		t.Errorf("ports: got %d in / %d out, want 1 in / 0 out (default sink)", info.Inputs, info.Outputs)
	}
}

func TestToBool_AcceptsStrings(t *testing.T) {
	cases := map[string]bool{
		"true": true, "True": true, "TRUE": true, "1": true,
		"false": false, "False": false, "FALSE": false, "0": false,
	}
	for s, want := range cases {
		got, err := toBool(s)
		if err != nil {
			t.Errorf("toBool(%q): unexpected error %v", s, err)
		}
		if got != want {
			t.Errorf("toBool(%q): got %v, want %v", s, got, want)
		}
	}
}

func TestToBool_RejectsBadString(t *testing.T) {
	if _, err := toBool("nope"); err == nil {
		t.Error("toBool(\"nope\") should error")
	}
}

func TestToByteSlice_AcceptsAllForms(t *testing.T) {
	want := []byte{0x01, 0x02, 0x03}
	cases := []any{
		[]byte{0x01, 0x02, 0x03},
		[]int{1, 2, 3},
		[]any{float64(1), float64(2), float64(3)},
		[]any{1, 2, 3},
	}
	for i, c := range cases {
		got, err := toByteSlice(c)
		if err != nil {
			t.Errorf("case %d (%T): %v", i, c, err)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("case %d: len %d, want %d", i, len(got), len(want))
			continue
		}
		for j := range want {
			if got[j] != want[j] {
				t.Errorf("case %d byte %d: got 0x%02x, want 0x%02x", i, j, got[j], want[j])
			}
		}
	}
}

func TestToByteSlice_RejectsOutOfRange(t *testing.T) {
	if _, err := toByteSlice([]int{0, 256}); err == nil {
		t.Error("toByteSlice([0, 256]) should error")
	}
	if _, err := toByteSlice([]int{-1, 0}); err == nil {
		t.Error("toByteSlice([-1, 0]) should error")
	}
}

// ─── Integration tests (require demo PLC) ─────────────────────────────────

type s7WriteHarness struct {
	t      *testing.T
	node   *S7WriteNode
	plc    *S7PLC
	mu     sync.Mutex
	sent   []*flow.Message
	status []struct{ fill, text string }
	errs   []error
}

func newS7WriteHarness(t *testing.T, props map[string]any) *s7WriteHarness {
	t.Helper()
	plc := requireDemoPLC(t)
	props["plc"] = "demo-plc"

	cfg := flow.NodeConfig{
		ID: t.Name(), Type: "s7-write", FlowID: "f1", Properties: props,
	}
	inst, err := NewS7WriteNode(cfg)
	if err != nil {
		t.Fatalf("NewS7WriteNode: %v", err)
	}
	n := inst.(*S7WriteNode)
	n.SetConfigLookup(func(id string) (flow.ConfigInstance, bool) {
		if id == "demo-plc" {
			return plc, true
		}
		return nil, false
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	h := &s7WriteHarness{t: t, node: n, plc: plc}
	n.SetSend(func(_ int, msg *flow.Message) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.sent = append(h.sent, msg)
	})
	n.SetStatus(func(fill, text string) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.status = append(h.status, struct{ fill, text string }{fill, text})
	})
	n.SetDebug(noopDebug)
	n.SetError(func(err error, _ *flow.Message) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.errs = append(h.errs, err)
	})
	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })
	return h
}

// readBackArea is a helper that reads N bytes from the area+address via the
// shared PLC manager — used by tests to verify writes landed.
func (h *s7WriteHarness) readBackArea(t *testing.T, area, db, start, length int) []byte {
	t.Helper()
	buf, err := h.plc.ReadArea(area, db, start, length)
	if err != nil {
		t.Fatalf("readBackArea: %v", err)
	}
	return buf
}

func TestS7Write_StaticValueFromMessage_Demo(t *testing.T) {
	// Static mode: variable's valueSource=msg with default valuePath=payload.
	// We send a payload, the node writes to MD4 (writable scratch in demo).
	h := newS7WriteHarness(t, map[string]any{
		"mode": "static",
		"variables": []any{
			map[string]any{
				"address":     "MD4",
				"dataType":    "dword",
				"valueSource": "msg",
			},
		},
		"emitAck": true,
	})

	in := flow.NewMessage()
	in.SetPayload(uint32(0x12345678))
	out, err := h.node.HandleMessage(in)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out) == 0 || len(out[0]) == 0 {
		t.Fatal("expected ACK output")
	}

	// Read back and verify.
	got := h.readBackArea(t, S7AreaMK, 0, 4, 4)
	want := []byte{0x12, 0x34, 0x56, 0x78}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestS7Write_StaticValueFromConfig_Demo(t *testing.T) {
	// Static mode, valueSource=static — value baked into config.
	h := newS7WriteHarness(t, map[string]any{
		"mode": "static",
		"variables": []any{
			map[string]any{
				"address":     "MD4",
				"dataType":    "dword",
				"valueSource": "static",
				"value":       float64(0xABCDEF01), // JSON-decoded → float64
			},
		},
	})
	if _, err := h.node.HandleMessage(flow.NewMessage()); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := h.readBackArea(t, S7AreaMK, 0, 4, 4)
	want := []byte{0xAB, 0xCD, 0xEF, 0x01}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestS7Write_ValuePathPicksFromNested_Demo(t *testing.T) {
	h := newS7WriteHarness(t, map[string]any{
		"mode": "static",
		"variables": []any{
			map[string]any{
				"address":     "MD4",
				"dataType":    "dword",
				"valueSource": "msg",
				"valuePath":   "payload.target",
			},
		},
	})

	in := flow.NewMessage()
	in.Set("payload", map[string]any{"target": float64(0xDEADBEEF)})
	if _, err := h.node.HandleMessage(in); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := h.readBackArea(t, S7AreaMK, 0, 4, 4)
	want := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestS7Write_DynamicMsgVariables_Demo(t *testing.T) {
	h := newS7WriteHarness(t, map[string]any{
		"mode":    "dynamic",
		"emitAck": true,
	})
	in := flow.NewMessage()
	in.Set("variables", []any{
		map[string]any{
			"address":  "MD4",
			"dataType": "dword",
			"value":    float64(0xCAFEBABE),
		},
	})
	out, err := h.node.HandleMessage(in)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out) == 0 || len(out[0]) == 0 {
		t.Fatal("expected ACK output")
	}
	if out[0][0].Payload() != true {
		t.Errorf("ACK payload: got %v, want true (allOk)", out[0][0].Payload())
	}

	got := h.readBackArea(t, S7AreaMK, 0, 4, 4)
	want := []byte{0xCA, 0xFE, 0xBA, 0xBE}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestS7Write_DynamicConvenienceForm_Demo(t *testing.T) {
	h := newS7WriteHarness(t, map[string]any{
		"mode":    "dynamic",
		"emitAck": true,
	})
	in := flow.NewMessage()
	in.Set("address", "MD4")
	in.Set("dataType", "dword")
	in.SetPayload(float64(0x11223344))
	if _, err := h.node.HandleMessage(in); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := h.readBackArea(t, S7AreaMK, 0, 4, 4)
	want := []byte{0x11, 0x22, 0x33, 0x44}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestS7Write_TypeMismatchSurfaces_Demo(t *testing.T) {
	// Sending a string where a real is expected → encode fails → per-item
	// error in the ACK. The whole transaction still succeeds (no transport
	// error escalates).
	h := newS7WriteHarness(t, map[string]any{
		"mode":    "dynamic",
		"emitAck": true,
	})
	in := flow.NewMessage()
	in.Set("variables", []any{
		map[string]any{
			"address":  "MD4",
			"dataType": "real",
			"value":    "this-is-not-a-number",
		},
	})
	out, err := h.node.HandleMessage(in)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out) == 0 || len(out[0]) == 0 {
		t.Fatal("expected ACK output even on type mismatch")
	}
	if out[0][0].Payload() != false {
		t.Errorf("ACK payload: got %v, want false (allOk = false)", out[0][0].Payload())
	}
	meta, _ := out[0][0].Get("s7Write").(map[string]any)
	results, _ := meta["results"].([]map[string]any)
	if len(results) != 1 || results[0]["error"] == nil {
		t.Errorf("expected error on item 0, got %+v", results)
	}
}

func TestS7Write_OutOfRangeSurfaces_Demo(t *testing.T) {
	h := newS7WriteHarness(t, map[string]any{
		"mode":    "dynamic",
		"emitAck": true,
	})
	in := flow.NewMessage()
	in.Set("variables", []any{
		map[string]any{
			"address":  "MB1",
			"dataType": "byte",
			"value":    float64(300), // out of [0, 255]
		},
	})
	out, _ := h.node.HandleMessage(in)
	meta, _ := out[0][0].Get("s7Write").(map[string]any)
	results, _ := meta["results"].([]map[string]any)
	errStr, _ := results[0]["error"].(string)
	if !strings.Contains(errStr, "out of range") {
		t.Errorf("expected 'out of range' error, got %q", errStr)
	}
}

func TestS7Write_InverseScaleApplied_Demo(t *testing.T) {
	// Variable scale=0.1, offset=5: input 25.0 → raw = (25 - 5) / 0.1 = 200.
	// Write to MW8 (signed int slot in the M area).
	h := newS7WriteHarness(t, map[string]any{
		"mode": "static",
		"variables": []any{
			map[string]any{
				"address":     "MW8",
				"dataType":    "int",
				"valueSource": "msg",
				"scale":       0.1,
				"offset":      5.0,
			},
		},
	})
	in := flow.NewMessage()
	in.SetPayload(float64(25.0))
	if _, err := h.node.HandleMessage(in); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := h.readBackArea(t, S7AreaMK, 0, 8, 2)
	// 200 in big-endian int16: 0x00, 0xC8
	if got[0] != 0x00 || got[1] != 0xC8 {
		t.Errorf("MW8 after inverse scaling: got [0x%02x, 0x%02x], want [0x00, 0xc8] (=200)", got[0], got[1])
	}
}

func TestS7Write_StringEncoded_Demo(t *testing.T) {
	// Write a STRING into a writable DB region. The demo's DB1 starts at 0
	// and STRING50.20 is initialised with "LOOPZE-S7-DEMO". Overwrite with
	// a shorter string, then read back and confirm the actLen byte updated.
	h := newS7WriteHarness(t, map[string]any{
		"mode":    "dynamic",
		"emitAck": true,
	})
	t.Cleanup(func() { restoreDemoDB1String(t, h.plc) })

	in := flow.NewMessage()
	in.Set("variables", []any{
		map[string]any{
			"address":  "DB1.STRING50.20",
			"dataType": "string",
			"value":    "TESTJOB",
		},
	})
	if _, err := h.node.HandleMessage(in); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := h.readBackArea(t, S7AreaDB, 1, 50, 22)
	if got[0] != 20 {
		t.Errorf("maxLen byte: got %d, want 20", got[0])
	}
	if got[1] != 7 {
		t.Errorf("actLen byte: got %d, want 7", got[1])
	}
	if string(got[2:9]) != "TESTJOB" {
		t.Errorf("chars: got %q, want %q", string(got[2:9]), "TESTJOB")
	}
}

func TestS7Write_PassthroughEnrichesInputMessage_Demo(t *testing.T) {
	h := newS7WriteHarness(t, map[string]any{
		"mode":        "dynamic",
		"passthrough": true,
	})
	in := flow.NewMessage()
	in.Set("origField", "preserved")
	in.Set("variables", []any{
		map[string]any{
			"address":  "MD4",
			"dataType": "dword",
			"value":    float64(0x01020304),
		},
	})
	out, err := h.node.HandleMessage(in)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out) == 0 || len(out[0]) == 0 {
		t.Fatal("expected passthrough output")
	}
	msg := out[0][0]
	// Original field preserved.
	if msg.Get("origField") != "preserved" {
		t.Errorf("origField: got %v, want preserved", msg.Get("origField"))
	}
	// s7Write metadata added.
	if meta, _ := msg.Get("s7Write").(map[string]any); meta == nil {
		t.Error("s7Write metadata missing on passthrough message")
	}
}

func TestS7Write_NoOutputWhenSilent_Demo(t *testing.T) {
	// Default: emitAck=false, passthrough=false → no output.
	h := newS7WriteHarness(t, map[string]any{
		"mode": "static",
		"variables": []any{
			map[string]any{
				"address":     "MD4",
				"dataType":    "dword",
				"valueSource": "msg",
			},
		},
	})
	in := flow.NewMessage()
	in.SetPayload(float64(0))
	out, err := h.node.HandleMessage(in)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected 0 outputs (sink), got %d", len(out))
	}
}

func TestS7Write_BoolWritesSingleBit_Demo(t *testing.T) {
	// Write M0.1 = true and verify only that bit is set in MB0 (the
	// heartbeat at M0.0 is animator-driven and may flip during the test, so
	// we only assert the bit-1 mask).
	h := newS7WriteHarness(t, map[string]any{
		"mode":    "dynamic",
		"emitAck": true,
	})
	in := flow.NewMessage()
	in.Set("variables", []any{
		map[string]any{
			"address":  "M0.1",
			"dataType": "bool",
			"value":    true,
		},
	})
	out, err := h.node.HandleMessage(in)
	if err != nil {
		// python-snap7 server doesn't support bit-level writes through
		// AGWriteMulti — it returns "function refused by CPU". Real Siemens
		// CPUs accept this; skip rather than fail.
		if strings.Contains(err.Error(), "function refused") {
			t.Skipf("python-snap7 demo server rejects bit-level multi-write (got %v); real Siemens CPUs handle this fine", err)
		}
		t.Fatalf("HandleMessage: %v", err)
	}
	if out[0][0].Payload() != true {
		t.Errorf("ACK: got %v, want true", out[0][0].Payload())
	}
	// The python-snap7 server may not preserve the heartbeat bit through a
	// write to a different bit (depends on impl); we only verify M0.1 is now
	// set, leaving the other bits unasserted.
	got := h.readBackArea(t, S7AreaMK, 0, 0, 1)
	if got[0]&(1<<1) == 0 {
		t.Errorf("M0.1 should be set; got byte 0x%02x", got[0])
	}
}

func TestS7Write_PerVariableErrorLeavesGoodOnesAlone_Demo(t *testing.T) {
	// One bad variable (out-of-range) and one good — the good one should
	// still hit the wire. Skip if multi-write is unsupported by the demo.
	h := newS7WriteHarness(t, map[string]any{
		"mode":    "dynamic",
		"emitAck": true,
	})
	in := flow.NewMessage()
	in.Set("variables", []any{
		map[string]any{
			"address":  "MB1",
			"dataType": "byte",
			"value":    float64(300), // bad: out of range
		},
		map[string]any{
			"address":  "MD4",
			"dataType": "dword",
			"value":    float64(0xFEED1234), // good
		},
	})
	out, err := h.node.HandleMessage(in)
	if err != nil {
		// If the demo can't handle multi-write either, skip.
		if strings.Contains(err.Error(), "invalid CPU answer") || strings.Contains(err.Error(), "Invalid CPU answer") {
			t.Skipf("python-snap7 demo doesn't fully support multi-write: %v", err)
		}
		t.Fatalf("HandleMessage: %v", err)
	}
	meta, _ := out[0][0].Get("s7Write").(map[string]any)
	results, _ := meta["results"].([]map[string]any)
	if results[0]["ok"] != false {
		t.Errorf("results[0].ok: got %v, want false (encode failed)", results[0]["ok"])
	}
	if results[1]["ok"] != true {
		t.Errorf("results[1].ok: got %v, want true (good item)", results[1]["ok"])
	}
	// Verify the good write actually landed.
	got := h.readBackArea(t, S7AreaMK, 0, 4, 4)
	want := []byte{0xFE, 0xED, 0x12, 0x34}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("MD4 byte %d: got 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

// ─── Block-mode integration tests ─────────────────────────────────────────

func TestS7Write_BlockStaticRoundTrip_Demo(t *testing.T) {
	// Write a 4-byte sentinel to MD4 via block mode (msg.payload = []byte),
	// then read it back via the manager and verify byte-perfect match.
	h := newS7WriteHarness(t, map[string]any{
		"mode": "block",
		"block": map[string]any{
			"area": "M", "start": 4, "length": 4,
		},
		"emitAck": true,
	})
	want := []byte{0xCA, 0xFE, 0xBA, 0xBE}
	in := flow.NewMessage()
	in.SetPayload(want)
	out, err := h.node.HandleMessage(in)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out) == 0 || out[0][0].Payload() != true {
		t.Errorf("expected ACK with payload=true, got %v", out)
	}

	got := h.readBackArea(t, S7AreaMK, 0, 4, 4)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestS7Write_BlockOversized_DB2_Demo(t *testing.T) {
	// 600-byte block exceeds the PDU payload, so the manager auto-splits
	// into ≥2 AGWriteDB calls. Write a deterministic ramp; verify by reading
	// it back. Round-trip catches concatenation mistakes on either side.
	h := newS7WriteHarness(t, map[string]any{
		"mode": "block",
		"block": map[string]any{
			"area": "DB", "db": 2, "start": 0, "length": 600,
		},
	})
	t.Cleanup(func() { restoreDemoDB2Ramp(t, h.plc) })
	want := make([]byte, 600)
	for i := range want {
		want[i] = byte((255 - i) & 0xFF) // descending ramp, distinct from the demo's pre-fill
	}
	in := flow.NewMessage()
	in.SetPayload(want)
	if _, err := h.node.HandleMessage(in); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	got := h.readBackArea(t, S7AreaDB, 2, 0, 600)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("byte %d: got 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestS7Write_BlockLengthMismatch_Demo(t *testing.T) {
	h := newS7WriteHarness(t, map[string]any{
		"mode": "block",
		"block": map[string]any{
			"area": "M", "start": 4, "length": 4,
		},
	})
	in := flow.NewMessage()
	in.SetPayload([]byte{0x01, 0x02}) // wrong length
	_, err := h.node.HandleMessage(in)
	if err == nil || !strings.Contains(err.Error(), "configured block.length") {
		t.Errorf("expected length-mismatch error, got %v", err)
	}
}

func TestS7Write_BlockCustomInputProperty_Demo(t *testing.T) {
	// Configure inputProperty=bytes (the convention the s7-parser would use
	// when piping its encoded output into s7-write).
	h := newS7WriteHarness(t, map[string]any{
		"mode": "block",
		"block": map[string]any{
			"area": "M", "start": 4, "length": 4,
			"inputProperty": "bytes",
		},
	})
	in := flow.NewMessage()
	in.Set("bytes", []byte{0xAA, 0xBB, 0xCC, 0xDD})
	if _, err := h.node.HandleMessage(in); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := h.readBackArea(t, S7AreaMK, 0, 4, 4)
	want := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestS7Write_BlockWithoutLengthUsesIncoming_Demo(t *testing.T) {
	// length omitted → take the incoming data length as-is. Useful for
	// variable-length payloads from the parser.
	h := newS7WriteHarness(t, map[string]any{
		"mode": "block",
		"block": map[string]any{
			"area": "M", "start": 4,
			// no length
		},
	})
	in := flow.NewMessage()
	in.SetPayload([]byte{0x10, 0x20, 0x30})
	if _, err := h.node.HandleMessage(in); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := h.readBackArea(t, S7AreaMK, 0, 4, 3)
	if got[0] != 0x10 || got[1] != 0x20 || got[2] != 0x30 {
		t.Errorf("got %v, want [0x10, 0x20, 0x30]", got)
	}
}

func TestS7Write_BlockDynamicOverride_Demo(t *testing.T) {
	// Configure DB1 / start=0, override per-message to write to DB2 / start=20.
	h := newS7WriteHarness(t, map[string]any{
		"mode": "block",
		"block": map[string]any{
			"area": "DB", "db": 1, "start": 0,
		},
	})
	t.Cleanup(func() { restoreDemoDB2Ramp(t, h.plc) })
	in := flow.NewMessage()
	in.Set("s7", map[string]any{
		"area": "DB", "db": 2, "start": 20,
	})
	in.SetPayload([]byte{0xEE, 0xFF})
	if _, err := h.node.HandleMessage(in); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := h.readBackArea(t, S7AreaDB, 2, 20, 2)
	if got[0] != 0xEE || got[1] != 0xFF {
		t.Errorf("DB2[20..22]: got %v, want [0xEE, 0xFF]", got)
	}
}

// restoreDemoDB1String resets DB1 STRING50.20 back to "LOOPZE-S7-DEMO" so
// later tests that read this string see the demo's documented value. The
// python-snap7 server keeps area state in memory; without this reset, write
// tests "infect" downstream read tests within the same `go test` invocation.
func restoreDemoDB1String(t *testing.T, plc *S7PLC) {
	t.Helper()
	wire := []byte{
		20, 14, // maxLen, actLen
		'L', 'O', 'O', 'P', 'Z', 'E', '-', 'S', '7', '-', 'D', 'E', 'M', 'O',
		0, 0, 0, 0, 0, 0,
	}
	if err := plc.WriteArea(S7AreaDB, 1, 50, wire); err != nil {
		t.Logf("restoreDemoDB1String: %v (ignored)", err)
	}
}

// restoreDemoDB2Ramp resets DB2 to the deterministic startup ramp
// `buf[i] = i & 0xFF`. Required after any write test that mutates DB2.
func restoreDemoDB2Ramp(t *testing.T, plc *S7PLC) {
	t.Helper()
	ramp := make([]byte, 600)
	for i := range ramp {
		ramp[i] = byte(i & 0xFF)
	}
	if err := plc.WriteArea(S7AreaDB, 2, 0, ramp); err != nil {
		t.Logf("restoreDemoDB2Ramp: %v (ignored)", err)
	}
}

// newS7WriteNode is a unit-test helper that builds an S7WriteNode without
// the engine wiring.
func newS7WriteNode(t *testing.T, props map[string]any) *S7WriteNode {
	t.Helper()
	cfg := flow.NodeConfig{
		ID: t.Name(), Type: "s7-write", FlowID: "f1", Properties: props,
	}
	inst, err := NewS7WriteNode(cfg)
	if err != nil {
		t.Fatalf("NewS7WriteNode: %v", err)
	}
	return inst.(*S7WriteNode)
}
