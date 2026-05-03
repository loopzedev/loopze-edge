// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"strings"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// makeParser constructs a parser node with the given properties already set.
// Callers run Init() and assert on the returned error.
func makeParser(props map[string]any) *ModbusParserNode {
	return &ModbusParserNode{
		config: flow.NodeConfig{
			ID:         "test-parser",
			Type:       "modbus-parser",
			Properties: props,
		},
	}
}

// field is a tiny helper to avoid repeating the verbose map[string]any literal
// in every test row.
func field(offset int, name, typ string, extra ...map[string]any) map[string]any {
	m := map[string]any{
		"offset": float64(offset),
		"name":   name,
		"type":   typ,
	}
	for _, e := range extra {
		for k, v := range e {
			m[k] = v
		}
	}
	return m
}

func TestParser_LayoutValidate_DuplicateName(t *testing.T) {
	n := makeParser(map[string]any{
		"layout": []any{
			field(0, "temperature", "float32"),
			field(2, "temperature", "uint16"),
		},
	})
	err := n.Init()
	if err == nil {
		t.Fatal("expected duplicate-name error")
	}
	if !strings.Contains(err.Error(), "duplicate field name") {
		t.Errorf("error should mention duplicate field name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "temperature") {
		t.Errorf("error should name the duplicated field, got: %v", err)
	}
}

func TestParser_LayoutValidate_StringNeedsLength(t *testing.T) {
	n := makeParser(map[string]any{
		"layout": []any{
			field(0, "tag", "string"),
		},
	})
	err := n.Init()
	if err == nil {
		t.Fatal("expected length-required error")
	}
	if !strings.Contains(err.Error(), "length") {
		t.Errorf("error should mention length, got: %v", err)
	}

	// raw without length is rejected the same way.
	n2 := makeParser(map[string]any{
		"layout": []any{
			field(0, "blob", "raw"),
		},
	})
	if err := n2.Init(); err == nil || !strings.Contains(err.Error(), "length") {
		t.Errorf("raw without length should error on length, got: %v", err)
	}
}

func TestParser_LayoutValidate_BitOutOfRange(t *testing.T) {
	cases := []struct {
		name string
		bit  int
	}{
		{"bit-16", 16},
		{"bit-100", 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := makeParser(map[string]any{
				"layout": []any{
					field(0, "fault", "bool", map[string]any{"bit": float64(tc.bit)}),
				},
			})
			err := n.Init()
			if err == nil {
				t.Fatal("expected bit-out-of-range error")
			}
			if !strings.Contains(err.Error(), "bit") {
				t.Errorf("error should mention bit, got: %v", err)
			}
		})
	}

	// Bit on a non-bool field is also rejected.
	n := makeParser(map[string]any{
		"layout": []any{
			field(0, "value", "uint16", map[string]any{"bit": float64(3)}),
		},
	})
	if err := n.Init(); err == nil || !strings.Contains(err.Error(), "bit") {
		t.Errorf("non-bool with bit should error, got: %v", err)
	}
}

func TestParser_LayoutValidate_RequiredFields(t *testing.T) {
	cases := []struct {
		name    string
		entry   map[string]any
		wantSub string
	}{
		{
			"missing name",
			map[string]any{"offset": float64(0), "type": "uint16"},
			"name is required",
		},
		{
			"missing type",
			map[string]any{"offset": float64(0), "name": "x"},
			"type is required",
		},
		{
			"unknown type",
			map[string]any{"offset": float64(0), "name": "x", "type": "doesNotExist"},
			"unknown type",
		},
		{
			"negative offset",
			map[string]any{"offset": float64(-1), "name": "x", "type": "uint16"},
			"offset must be",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := makeParser(map[string]any{"layout": []any{tc.entry}})
			err := n.Init()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error should contain %q, got: %v", tc.wantSub, err)
			}
		})
	}
}

func TestParser_LayoutValidate_EmptyLayout(t *testing.T) {
	n := makeParser(map[string]any{"layout": []any{}})
	err := n.Init()
	if err == nil {
		t.Fatal("expected error on empty layout")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention empty layout, got: %v", err)
	}
}

func TestParser_LayoutValidate_OK_Mixed(t *testing.T) {
	n := makeParser(map[string]any{
		"action":    "auto",
		"byteOrder": "bigEndian",
		"wordOrder": "bigEndian",
		"layout": []any{
			field(0, "temperature", "float32", map[string]any{"scale": 1.0, "unit": "°C"}),
			field(2, "counter", "uint32"),
			field(4, "pressure", "float32", map[string]any{"scale": 0.1, "unit": "bar"}),
			field(6, "setpoint", "int16"),
			field(7, "mode", "uint16"),
			field(10, "tag", "string", map[string]any{"length": float64(5)}),
			field(20, "energy", "float32", map[string]any{"scale": 0.001, "unit": "kWh"}),
			// Bit fields on the same register — legitimate, must not error.
			field(8, "motor_running", "bool", map[string]any{"bit": float64(0)}),
			field(8, "fault", "bool", map[string]any{"bit": float64(1)}),
			field(8, "maintenance", "bool", map[string]any{"bit": float64(7)}),
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(n.layout) != 10 {
		t.Errorf("expected 10 fields, got %d", len(n.layout))
	}
	if n.byteOrder != ByteOrderBig {
		t.Errorf("expected default byte order big, got %v", n.byteOrder)
	}
	if n.action != "auto" {
		t.Errorf("expected action=auto, got %v", n.action)
	}
}

func TestParser_LayoutValidate_OverlapWarn(t *testing.T) {
	// Two non-bit fields claiming overlapping registers — warning only, no error.
	// This is intentionally permitted (e.g. inspecting the same register two
	// different ways during commissioning).
	n := makeParser(map[string]any{
		"layout": []any{
			field(0, "as_uint32", "uint32"),
			field(1, "as_uint16", "uint16"), // overlaps register 1
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("overlap should warn, not error, got: %v", err)
	}
	if len(n.layout) != 2 {
		t.Errorf("expected both fields kept after overlap warn, got %d", len(n.layout))
	}
}

func TestParser_LayoutValidate_DefaultsApplied(t *testing.T) {
	// Layout entry with minimal fields — defaults should fill in.
	n := makeParser(map[string]any{
		"layout": []any{
			map[string]any{
				"offset": float64(0),
				"name":   "x",
				"type":   "uint16",
			},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	f := n.layout[0]
	if f.Scale != 1 {
		t.Errorf("default scale should be 1, got %v", f.Scale)
	}
	if f.Bit != -1 {
		t.Errorf("default bit should be -1 (unset), got %v", f.Bit)
	}
	if f.OffsetValue != 0 {
		t.Errorf("default offsetValue should be 0, got %v", f.OffsetValue)
	}
}

func TestParser_ActionFallback(t *testing.T) {
	// Unknown action falls back to "auto" with a warning.
	n := makeParser(map[string]any{
		"action": "bogus",
		"layout": []any{field(0, "x", "uint16")},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if n.action != "auto" {
		t.Errorf("unknown action should fall back to auto, got %q", n.action)
	}
}

// ── Parse / Encode / Roundtrip ───────────────────────────────────────────────

// initParser is a small helper that builds a parser, runs Init, and fails the
// test fatally on validation errors. Returns the prepared node.
func initParser(t *testing.T, props map[string]any) *ModbusParserNode {
	t.Helper()
	n := makeParser(props)
	if err := n.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	return n
}

// regsFromValues encodes a list of (type, value) pairs into a register slice
// using the codec — useful as fixture for parse tests.
func regsFromValues(t *testing.T, items []struct {
	dtype string
	value any
}) []uint16 {
	t.Helper()
	var out []uint16
	for _, it := range items {
		regs, err := EncodeRegisters(it.dtype, ByteOrderBig, WordOrderBig, it.value)
		if err != nil {
			t.Fatalf("fixture encode %s=%v: %v", it.dtype, it.value, err)
		}
		out = append(out, regs...)
	}
	return out
}

func TestParser_Parse_AllTypes(t *testing.T) {
	n := initParser(t, map[string]any{
		"layout": []any{
			field(0, "f32", "float32"),
			field(2, "u32", "uint32"),
			field(4, "i16", "int16"),
			field(5, "u16", "uint16"),
			field(6, "tag", "string", map[string]any{"length": float64(3)}),
			field(9, "f64", "float64"),
			field(13, "i64", "int64"),
			field(17, "raw", "raw", map[string]any{"length": float64(2)}),
			field(19, "u64", "uint64"),
		},
	})
	regs := regsFromValues(t, []struct {
		dtype string
		value any
	}{
		{"float32", float32(21.5)},
		{"uint32", int64(123456)},
		{"int16", -123},
		{"uint16", 42},
		{"string", "AB" + "\x00\x00\x00\x00"}, // 3 regs = 6 bytes
		{"float64", float64(3.14159265358979)},
		{"int64", int64(-9876543210)},
		{"raw", []uint16{0xCAFE, 0xBABE}},
		{"uint64", uint64(0x0102030405060708)},
	})

	out, err := n.parse(regs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Spot-check each typed field.
	if v := out["f32"].(float64); v < 21.4 || v > 21.6 {
		t.Errorf("f32: got %v, want ~21.5", v)
	}
	if v := out["u32"].(int64); v != 123456 {
		t.Errorf("u32: got %v, want 123456", v)
	}
	if v := out["i16"].(int); v != -123 {
		t.Errorf("i16: got %v, want -123", v)
	}
	if v := out["u16"].(int); v != 42 {
		t.Errorf("u16: got %v, want 42", v)
	}
	if v := out["tag"].(string); v != "AB" {
		t.Errorf("tag: got %q, want %q", v, "AB")
	}
	if v := out["f64"].(float64); v < 3.14159 || v > 3.14160 {
		t.Errorf("f64: got %v, want ~π", v)
	}
	if v := out["i64"].(int64); v != -9876543210 {
		t.Errorf("i64: got %v, want -9876543210", v)
	}
	if v := out["u64"].(uint64); v != 0x0102030405060708 {
		t.Errorf("u64: got %#x, want 0x0102030405060708", v)
	}
	if v := out["raw"].([]int); len(v) != 2 || v[0] != 0xCAFE || v[1] != 0xBABE {
		t.Errorf("raw: got %v, want [0xCAFE, 0xBABE]", v)
	}
}

func TestParser_Parse_SparseLayout(t *testing.T) {
	// Holes between fields (offsets 0, 4, 10) — parser must address each
	// field at the right slice without depending on contiguous layout.
	n := initParser(t, map[string]any{
		"layout": []any{
			field(0, "a", "uint16"),
			field(4, "b", "float32"),
			field(10, "c", "int16"),
		},
	})

	// Build 12 registers; only positions 0, 4..5, 10 are set, rest = 0.
	regs := make([]uint16, 12)
	regs[0] = 0xABCD
	floatRegs, _ := EncodeRegisters("float32", ByteOrderBig, WordOrderBig, float32(1.5))
	regs[4] = floatRegs[0]
	regs[5] = floatRegs[1]
	int16Regs, _ := EncodeRegisters("int16", ByteOrderBig, WordOrderBig, -42)
	regs[10] = int16Regs[0]

	out, err := n.parse(regs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out["a"].(int) != 0xABCD {
		t.Errorf("a: got %v, want 0xABCD", out["a"])
	}
	if v := out["b"].(float64); v < 1.49 || v > 1.51 {
		t.Errorf("b: got %v, want 1.5", v)
	}
	if out["c"].(int) != -42 {
		t.Errorf("c: got %v, want -42", out["c"])
	}
}

func TestParser_Parse_InsufficientInput(t *testing.T) {
	n := initParser(t, map[string]any{
		"layout": []any{
			field(0, "x", "uint16"),
			field(1, "y", "float32"), // needs 2 regs at offset 1, so total 3
		},
	})
	_, err := n.parse([]uint16{0x1234, 0x5678}) // only 2 regs → too short
	if err == nil {
		t.Fatal("expected error for short input")
	}
	if !strings.Contains(err.Error(), "y") {
		t.Errorf("error should mention failing field 'y', got: %v", err)
	}
}

func TestParser_Parse_PerFieldByteOrderOverride(t *testing.T) {
	// Default node order = big/big. Field "swapped" overrides to little byte order.
	n := initParser(t, map[string]any{
		"byteOrder": "bigEndian",
		"wordOrder": "bigEndian",
		"layout": []any{
			field(0, "default", "uint16"),
			field(1, "swapped", "uint16", map[string]any{"byteOrder": "littleEndian"}),
		},
	})
	// Both registers carry the same wire bits 0x4142 ("AB" big-endian).
	regs := []uint16{0x4142, 0x4142}
	out, err := n.parse(regs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Default (big-endian) reads 0x4142 = 16706.
	if out["default"].(int) != 0x4142 {
		t.Errorf("default: got %v, want 0x4142", out["default"])
	}
	// Little-endian byte order: high/low byte swapped → 0x4241.
	if out["swapped"].(int) != 0x4241 {
		t.Errorf("swapped: got %v, want 0x4241", out["swapped"])
	}
}

func TestParser_Parse_BitFields(t *testing.T) {
	n := initParser(t, map[string]any{
		"layout": []any{
			field(0, "running", "bool", map[string]any{"bit": float64(0)}),
			field(0, "fault", "bool", map[string]any{"bit": float64(1)}),
			field(0, "maint", "bool", map[string]any{"bit": float64(7)}),
			field(1, "whole_reg_bool", "bool"), // no bit → whole register
		},
	})
	// Bits 0 and 7 set, bit 1 clear → 0b10000001 = 0x81. Whole-reg = 1 (true).
	regs := []uint16{0x0081, 0x0001}
	out, err := n.parse(regs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !out["running"].(bool) {
		t.Errorf("running: expected true (bit 0 set)")
	}
	if out["fault"].(bool) {
		t.Errorf("fault: expected false (bit 1 clear)")
	}
	if !out["maint"].(bool) {
		t.Errorf("maint: expected true (bit 7 set)")
	}
	// Whole-register bool (no bit) returns true if the register is non-zero.
	if !out["whole_reg_bool"].(bool) {
		t.Errorf("whole_reg_bool: expected true (register == 1)")
	}
}

func TestParser_Parse_Scale(t *testing.T) {
	// Pressure stored as int16 in centibar; layout scales to bar.
	n := initParser(t, map[string]any{
		"layout": []any{
			field(0, "pressure", "int16", map[string]any{"scale": 0.01}),
			field(1, "tempOffset", "int16", map[string]any{"scale": 0.1, "offsetValue": -40.0}),
		},
	})
	regs := []uint16{
		uint16(int16(102)),  // 102 * 0.01 = 1.02 bar
		uint16(int16(1234)), // 1234 * 0.1 + (-40) = 83.4 °C
	}
	out, err := n.parse(regs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v := out["pressure"].(float64); v < 1.01 || v > 1.03 {
		t.Errorf("pressure: got %v, want ~1.02", v)
	}
	if v := out["tempOffset"].(float64); v < 83.39 || v > 83.41 {
		t.Errorf("tempOffset: got %v, want ~83.4", v)
	}
}

func TestParser_Encode_Basic(t *testing.T) {
	n := initParser(t, map[string]any{
		"layout": []any{
			field(0, "temp", "float32"),
			field(2, "count", "uint32"),
		},
	})
	regs, minOffset, err := n.encode(map[string]any{
		"temp":  float64(21.5),
		"count": int64(123456),
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if minOffset != 0 {
		t.Errorf("minOffset: got %d, want 0", minOffset)
	}
	if len(regs) != 4 {
		t.Errorf("regs length: got %d, want 4", len(regs))
	}

	// Roundtrip.
	out, err := n.parse(regs)
	if err != nil {
		t.Fatalf("parse roundtrip: %v", err)
	}
	if v := out["temp"].(float64); v < 21.49 || v > 21.51 {
		t.Errorf("roundtrip temp: got %v", v)
	}
	if out["count"].(int64) != 123456 {
		t.Errorf("roundtrip count: got %v", out["count"])
	}
}

func TestParser_Encode_Sparse(t *testing.T) {
	n := initParser(t, map[string]any{
		"layout": []any{
			field(0, "a", "uint16"),
			field(1, "b", "uint16"),
			field(2, "c", "uint16"),
			field(3, "d", "uint16"),
			field(4, "e", "uint16"),
		},
	})
	// Only set 'b' and 'd' — others must be zero.
	regs, _, err := n.encode(map[string]any{
		"b": 100,
		"d": 200,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	want := []uint16{0, 100, 0, 200, 0}
	if len(regs) != len(want) {
		t.Fatalf("length: got %d, want %d", len(regs), len(want))
	}
	for i, w := range want {
		if regs[i] != w {
			t.Errorf("regs[%d]: got %d, want %d", i, regs[i], w)
		}
	}
}

func TestParser_Encode_BitFieldsOR(t *testing.T) {
	// Three bit fields on offset 0 — when all true, register must have all
	// three bits set.
	n := initParser(t, map[string]any{
		"layout": []any{
			field(0, "running", "bool", map[string]any{"bit": float64(0)}),
			field(0, "fault", "bool", map[string]any{"bit": float64(1)}),
			field(0, "maint", "bool", map[string]any{"bit": float64(7)}),
		},
	})
	regs, _, err := n.encode(map[string]any{
		"running": true,
		"fault":   true,
		"maint":   true,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(regs) != 1 {
		t.Fatalf("regs length: got %d, want 1", len(regs))
	}
	want := uint16(0b10000011) // bits 0, 1, 7
	if regs[0] != want {
		t.Errorf("regs[0]: got %#08b, want %#08b", regs[0], want)
	}

	// Subset: only "fault" set.
	regs2, _, err := n.encode(map[string]any{"fault": true})
	if err != nil {
		t.Fatalf("encode subset: %v", err)
	}
	if regs2[0] != 0b10 {
		t.Errorf("subset regs[0]: got %#08b, want 0b10", regs2[0])
	}
}

func TestParser_Encode_ScaleInverse(t *testing.T) {
	// pressure stored as int16 centibar; layout scales 0.01.
	// Input 1.024 bar → encoded raw = 1.024 / 0.01 = 102 (rounded).
	n := initParser(t, map[string]any{
		"layout": []any{
			field(0, "pressure", "int16", map[string]any{"scale": 0.01}),
		},
	})
	regs, _, err := n.encode(map[string]any{"pressure": 1.02})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if int16(regs[0]) != 102 {
		t.Errorf("scaled encode: got %d, want 102", int16(regs[0]))
	}
}

func TestParser_Encode_EmptyInput(t *testing.T) {
	n := initParser(t, map[string]any{
		"layout": []any{field(0, "x", "uint16")},
	})
	if _, _, err := n.encode(map[string]any{}); err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParser_Encode_MinOffset(t *testing.T) {
	// Lowest offset in the layout must be reported as the encode "address" hint.
	n := initParser(t, map[string]any{
		"layout": []any{
			field(10, "a", "uint16"),
			field(20, "b", "uint16"),
			field(5, "c", "uint16"),
		},
	})
	_, minOffset, err := n.encode(map[string]any{"a": 1, "b": 2, "c": 3})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if minOffset != 5 {
		t.Errorf("minOffset: got %d, want 5", minOffset)
	}
}

// ── Action dispatch ──────────────────────────────────────────────────────────

func TestParser_AutoDispatch_Map_Encodes(t *testing.T) {
	n := initParser(t, map[string]any{
		"action": "auto",
		"layout": []any{field(0, "x", "uint16"), field(1, "y", "uint16")},
	})
	msg := flow.NewMessage()
	msg.SetPayload(map[string]any{"x": 11, "y": 22})

	out, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(out) != 1 || len(out[0]) != 1 {
		t.Fatalf("expected one message, got %v", out)
	}

	got := out[0][0].Payload().([]int)
	if len(got) != 2 || got[0] != 11 || got[1] != 22 {
		t.Errorf("payload: got %v, want [11, 22]", got)
	}
	bytes := out[0][0].Get("bytes").([]int)
	if len(bytes) != 4 {
		t.Errorf("bytes length: got %d, want 4", len(bytes))
	}
	if addr := out[0][0].Get("address"); addr != 0 {
		t.Errorf("address: got %v, want 0", addr)
	}
}

func TestParser_AutoDispatch_RegisterArray_Parses(t *testing.T) {
	n := initParser(t, map[string]any{
		"action":    "auto",
		"parseFrom": "bytes",
		"layout":    []any{field(0, "x", "uint16"), field(1, "y", "uint16")},
	})
	msg := flow.NewMessage()
	// modbus-read with raw bytes ([]int range 0..255) → 4 bytes for 2 regs.
	msg.Set("bytes", []int{0x12, 0x34, 0x56, 0x78})

	out, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	got := out[0][0].Payload().(map[string]any)
	if got["x"].(int) != 0x1234 {
		t.Errorf("x: got %v, want 0x1234", got["x"])
	}
	if got["y"].(int) != 0x5678 {
		t.Errorf("y: got %v, want 0x5678", got["y"])
	}
}

func TestParser_AutoDispatch_WordArray_Parses(t *testing.T) {
	// When the user pipes msg.payload (word array, []int with values > 255)
	// into parseFrom, the heuristic must treat it as registers, not bytes.
	n := initParser(t, map[string]any{
		"action":    "auto",
		"parseFrom": "payload",
		"layout":    []any{field(0, "v", "float32")},
	})
	msg := flow.NewMessage()
	// 21.5 in float32 BE = 0x41AC0000 → regs [0x41AC, 0x0000] → ints with one > 255.
	msg.SetPayload([]int{0x41AC, 0x0000})

	out, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	got := out[0][0].Payload().(map[string]any)
	if v := got["v"].(float64); v < 21.49 || v > 21.51 {
		t.Errorf("v: got %v, want 21.5", v)
	}
}

func TestParser_ParseAction_RejectsMap(t *testing.T) {
	n := initParser(t, map[string]any{
		"action": "parse",
		"layout": []any{field(0, "x", "uint16")},
	})
	msg := flow.NewMessage()
	msg.Set("bytes", map[string]any{"x": 1}) // wrong shape for parse
	if _, err := n.HandleMessage(msg); err == nil {
		t.Fatal("expected parse error on map input")
	}
}

func TestParser_EncodeAction_RejectsArray(t *testing.T) {
	n := initParser(t, map[string]any{
		"action": "encode",
		"layout": []any{field(0, "x", "uint16")},
	})
	msg := flow.NewMessage()
	msg.SetPayload([]int{1, 2, 3}) // wrong shape for encode
	if _, err := n.HandleMessage(msg); err == nil {
		t.Fatal("expected encode error on array input")
	}
}

func TestParser_StartClearsStaleStatus(t *testing.T) {
	// After a redeploy the node is fresh (inErrorState=false) but the
	// frontend may still show a red pill from the previous incarnation.
	// Start() must explicitly clear the status pill once.
	statusCalls := []struct{ fill, text string }{}
	n := initParser(t, map[string]any{
		"layout": []any{field(0, "x", "uint16")},
	})
	n.SetStatus(func(fill, text string) {
		statusCalls = append(statusCalls, struct{ fill, text string }{fill, text})
	})

	if err := n.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(statusCalls) != 1 {
		t.Fatalf("expected 1 status call from Start, got %d: %v", len(statusCalls), statusCalls)
	}
	if statusCalls[0].fill != "" || statusCalls[0].text != "" {
		t.Errorf("Start should clear (empty fill+text), got %v", statusCalls[0])
	}
}

func TestParser_StatusErrorRecovery(t *testing.T) {
	// First message fails, second succeeds — the node must clear the error
	// status pill on recovery.
	statusCalls := []struct {
		fill string
		text string
	}{}
	n := initParser(t, map[string]any{
		"action": "parse",
		"layout": []any{field(0, "x", "uint16")},
	})
	n.SetStatus(func(fill, text string) {
		statusCalls = append(statusCalls, struct{ fill, text string }{fill, text})
	})

	// Failing call.
	msg1 := flow.NewMessage()
	msg1.Set("bytes", "not a buffer")
	if _, err := n.HandleMessage(msg1); err == nil {
		t.Fatal("expected error")
	}
	// Successful call clears.
	msg2 := flow.NewMessage()
	msg2.Set("bytes", []int{0x00, 0x05})
	if _, err := n.HandleMessage(msg2); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	if len(statusCalls) < 2 {
		t.Fatalf("expected 2 status calls, got %d: %v", len(statusCalls), statusCalls)
	}
	if statusCalls[0].fill != "red" {
		t.Errorf("first call should set red, got %v", statusCalls[0])
	}
	if statusCalls[1].fill != "" {
		t.Errorf("recovery call should clear (empty fill), got %v", statusCalls[1])
	}
}

func TestParser_RoundTrip_AllTypes(t *testing.T) {
	n := initParser(t, map[string]any{
		"layout": []any{
			field(0, "f32", "float32"),
			field(2, "u32", "uint32"),
			field(4, "i16", "int16"),
			field(5, "u16", "uint16"),
			field(6, "i32", "int32"),
			field(8, "f64", "float64"),
			field(12, "i64", "int64"),
			// bit fields — non-zero, non-zero, zero
			field(16, "running", "bool", map[string]any{"bit": float64(0)}),
			field(16, "fault", "bool", map[string]any{"bit": float64(1)}),
			field(16, "maint", "bool", map[string]any{"bit": float64(7)}),
		},
	})

	original := map[string]any{
		"f32":     1.5,
		"u32":     int64(0xDEADBEEF),
		"i16":     -1234,
		"u16":     65000,
		"i32":     int64(-100000),
		"f64":     2.71828,
		"i64":     int64(-9876543210),
		"running": true,
		"fault":   false,
		"maint":   true,
	}

	regs, _, err := n.encode(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := n.parse(regs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	checkFloat := func(name string, want float64, tol float64) {
		v, ok := out[name].(float64)
		if !ok {
			t.Errorf("%s: not float64, got %T", name, out[name])
			return
		}
		if v < want-tol || v > want+tol {
			t.Errorf("%s: got %v, want %v ±%v", name, v, want, tol)
		}
	}
	checkFloat("f32", 1.5, 1e-5)
	checkFloat("f64", 2.71828, 1e-9)

	if out["u32"].(int64) != 0xDEADBEEF {
		t.Errorf("u32: got %v", out["u32"])
	}
	if out["i16"].(int) != -1234 {
		t.Errorf("i16: got %v", out["i16"])
	}
	if out["u16"].(int) != 65000 {
		t.Errorf("u16: got %v", out["u16"])
	}
	if out["i32"].(int) != -100000 {
		t.Errorf("i32: got %v", out["i32"])
	}
	if out["i64"].(int64) != -9876543210 {
		t.Errorf("i64: got %v", out["i64"])
	}
	if out["running"].(bool) != true {
		t.Errorf("running: got %v", out["running"])
	}
	if out["fault"].(bool) != false {
		t.Errorf("fault: got %v", out["fault"])
	}
	if out["maint"].(bool) != true {
		t.Errorf("maint: got %v", out["maint"])
	}
}
