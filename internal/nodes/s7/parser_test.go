// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package s7

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// ─── Layout validation ────────────────────────────────────────────────────

func TestS7Parser_Init_RejectsEmptyLayout(t *testing.T) {
	cases := []map[string]any{
		{},
		{"layout": []any{}},
	}
	for _, props := range cases {
		n := newS7ParserNode(t, props)
		if err := n.Init(); err == nil {
			t.Errorf("Init should reject empty layout (props=%v)", props)
		}
	}
}

func TestS7Parser_Init_RejectsLayoutNotArray(t *testing.T) {
	n := newS7ParserNode(t, map[string]any{
		"layout": "this is not an array",
	})
	if err := n.Init(); err == nil {
		t.Fatal("Init should reject non-array layout")
	}
}

func TestS7Parser_Init_RejectsDuplicateNames(t *testing.T) {
	n := newS7ParserNode(t, map[string]any{
		"layout": []any{
			map[string]any{"offset": 0, "name": "a", "type": "byte"},
			map[string]any{"offset": 1, "name": "a", "type": "byte"},
		},
	})
	if err := n.Init(); err == nil || !strings.Contains(err.Error(), "duplicate field name") {
		t.Errorf("expected duplicate-name error, got %v", err)
	}
}

func TestS7Parser_Init_RejectsMissingFields(t *testing.T) {
	cases := []struct {
		name  string
		entry map[string]any
		hint  string
	}{
		{"missing offset", map[string]any{"name": "a", "type": "byte"}, "offset is required"},
		{"missing name", map[string]any{"offset": 0, "type": "byte"}, "name is required"},
		{"missing type", map[string]any{"offset": 0, "name": "a"}, "type is required"},
		{"unknown type", map[string]any{"offset": 0, "name": "a", "type": "fancy"}, "unknown type"},
		{"string without length", map[string]any{"offset": 0, "name": "s", "type": "string"}, "length"},
		{"raw without length", map[string]any{"offset": 0, "name": "r", "type": "raw"}, "length"},
		{"string maxLen 255", map[string]any{"offset": 0, "name": "s", "type": "string", "length": 255}, "[1, 254]"},
		{"signed on inherently-signed type", map[string]any{"offset": 0, "name": "i", "type": "int", "signed": true}, "signed flag only meaningful"},
		{"bit on non-bool type", map[string]any{"offset": "0.3", "name": "x", "type": "byte"}, "dotted offset"},
		{"bit out of range", map[string]any{"offset": "0.8", "name": "x", "type": "bool"}, "[0, 7]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := newS7ParserNode(t, map[string]any{
				"layout": []any{c.entry},
			})
			err := n.Init()
			if err == nil {
				t.Fatalf("Init should reject %v", c.entry)
			}
			if !strings.Contains(err.Error(), c.hint) {
				t.Errorf("error %q does not contain %q", err.Error(), c.hint)
			}
		})
	}
}

func TestS7Parser_Init_RejectsBlockLengthSmallerThanLayout(t *testing.T) {
	n := newS7ParserNode(t, map[string]any{
		"blockLength": 4,
		"layout": []any{
			map[string]any{"offset": 8, "name": "x", "type": "real"}, // ends at byte 12
		},
	})
	if err := n.Init(); err == nil || !strings.Contains(err.Error(), "max extent") {
		t.Errorf("expected blockLength/extent error, got %v", err)
	}
}

func TestParseS7Offset(t *testing.T) {
	cases := []struct {
		in       any
		wantByte int
		wantBit  int
		errLike  string
	}{
		{12, 12, 0, ""},
		{int64(5), 5, 0, ""},
		{float64(7), 7, 0, ""},
		{"12", 12, 0, ""},
		{"12.3", 12, 3, ""},
		{"  100.7  ", 100, 7, ""},
		{"12.8", 0, 0, "[0, 7]"},
		{"abc", 0, 0, ""},     // strconv error
		{nil, 0, 0, "must be"}, // unsupported type
	}
	for _, c := range cases {
		byteOff, bitOff, err := parseS7Offset(c.in)
		if c.errLike != "" {
			if err == nil || !strings.Contains(err.Error(), c.errLike) {
				t.Errorf("parseS7Offset(%v) error: got %v, want substring %q", c.in, err, c.errLike)
			}
			continue
		}
		if c.in == "abc" {
			if err == nil {
				t.Errorf("parseS7Offset(\"abc\") should error")
			}
			continue
		}
		if err != nil {
			t.Errorf("parseS7Offset(%v): %v", c.in, err)
			continue
		}
		if byteOff != c.wantByte || bitOff != c.wantBit {
			t.Errorf("parseS7Offset(%v) = (%d, %d), want (%d, %d)", c.in, byteOff, bitOff, c.wantByte, c.wantBit)
		}
	}
}

// ─── Parse path ────────────────────────────────────────────────────────────

func TestS7Parser_Parse_DemoDB1(t *testing.T) {
	// Build a buffer that mirrors the demo's DB1 layout. Then parse and
	// confirm the values come back. Uses real S7 wire bytes for known
	// values to catch endianness bugs.
	buf := make([]byte, 80)
	// Temperature at DBD0 = REAL(21.5) → 0x41AC0000
	buf[0], buf[1], buf[2], buf[3] = 0x41, 0xAC, 0x00, 0x00
	// Tick at DBD8 = DINT(1234) → 0x000004D2
	buf[8], buf[9], buf[10], buf[11] = 0x00, 0x00, 0x04, 0xD2
	// Setpoint at DBW12 = INT(200) → 0x00C8
	buf[12], buf[13] = 0x00, 0xC8
	// AlarmActive at byte 16 bit 0 = true → 0x01
	buf[16] = 0x01
	// STRING50.20 = "TESTJOB" — header (20, 7) + 7 chars
	buf[50], buf[51] = 20, 7
	copy(buf[52:], []byte("TESTJOB"))

	n := newS7ParserNode(t, map[string]any{
		"layout": []any{
			map[string]any{"offset": 0, "name": "Temperature", "type": "real"},
			map[string]any{"offset": 8, "name": "Tick", "type": "dint"},
			map[string]any{"offset": 12, "name": "Setpoint", "type": "int"},
			map[string]any{"offset": "16.0", "name": "AlarmActive", "type": "bool"},
			map[string]any{"offset": 50, "name": "Tag", "type": "string", "length": 20},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	result, err := n.parse(buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result["Temperature"] != float32(21.5) {
		t.Errorf("Temperature: got %v (%T), want 21.5", result["Temperature"], result["Temperature"])
	}
	if result["Tick"] != int32(1234) {
		t.Errorf("Tick: got %v (%T), want 1234", result["Tick"], result["Tick"])
	}
	if result["Setpoint"] != int16(200) {
		t.Errorf("Setpoint: got %v (%T), want 200", result["Setpoint"], result["Setpoint"])
	}
	if result["AlarmActive"] != true {
		t.Errorf("AlarmActive: got %v, want true", result["AlarmActive"])
	}
	if result["Tag"] != "TESTJOB" {
		t.Errorf("Tag: got %q, want %q", result["Tag"], "TESTJOB")
	}
}

func TestS7Parser_Parse_BoolBitsAtSameByte(t *testing.T) {
	// One byte holding 8 packed status flags. Layout binds names to the
	// individual bit positions; parse should return all 8 bools.
	buf := []byte{0b10110011} // bits 0,1,4,5,7 set
	layout := []any{
		map[string]any{"offset": "0.0", "name": "alarm", "type": "bool"},
		map[string]any{"offset": "0.1", "name": "running", "type": "bool"},
		map[string]any{"offset": "0.2", "name": "fault", "type": "bool"},
		map[string]any{"offset": "0.3", "name": "manual", "type": "bool"},
		map[string]any{"offset": "0.4", "name": "auto", "type": "bool"},
		map[string]any{"offset": "0.5", "name": "ready", "type": "bool"},
		map[string]any{"offset": "0.6", "name": "warning", "type": "bool"},
		map[string]any{"offset": "0.7", "name": "maintenance", "type": "bool"},
	}
	n := newS7ParserNode(t, map[string]any{"layout": layout})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	got, err := n.parse(buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := map[string]bool{
		"alarm": true, "running": true, "fault": false, "manual": false,
		"auto": true, "ready": true, "warning": false, "maintenance": true,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %v, want %v (byte 0x%02b)", k, got[k], v, buf[0])
		}
	}
}

func TestS7Parser_Parse_AppliesScale(t *testing.T) {
	buf := []byte{0x00, 0xC8} // INT 200
	n := newS7ParserNode(t, map[string]any{
		"layout": []any{
			map[string]any{"offset": 0, "name": "x", "type": "int", "scale": 0.1, "valueOffset": 5.0},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	got, err := n.parse(buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v, ok := got["x"].(float64); !ok || v < 24.99 || v > 25.01 {
		t.Errorf("scaled value: got %v (%T), want ~25.0", got["x"], got["x"])
	}
}

func TestS7Parser_Parse_BufferTooShort(t *testing.T) {
	buf := []byte{0x01, 0x02}
	n := newS7ParserNode(t, map[string]any{
		"layout": []any{
			map[string]any{"offset": 0, "name": "x", "type": "real"}, // needs 4 bytes
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_, err := n.parse(buf)
	if err == nil || !strings.Contains(err.Error(), "needs 4 byte") {
		t.Errorf("expected too-short error, got %v", err)
	}
}

func TestS7Parser_Parse_RawCopy(t *testing.T) {
	// raw should return an independent copy (mutating the result must not
	// affect the source).
	buf := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE}
	n := newS7ParserNode(t, map[string]any{
		"layout": []any{
			map[string]any{"offset": 1, "name": "blob", "type": "raw", "length": 4},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	got, err := n.parse(buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	blob := got["blob"].([]byte)
	want := []byte{0xAD, 0xBE, 0xEF, 0xCA}
	if !bytes.Equal(blob, want) {
		t.Errorf("blob: got %v, want %v", blob, want)
	}
	blob[0] = 0xFF
	if buf[1] == 0xFF {
		t.Error("raw decode aliased the source buffer; expected an independent copy")
	}
}

// ─── Encode path ───────────────────────────────────────────────────────────

func TestS7Parser_Encode_Sparse(t *testing.T) {
	// Only the named field is in the input — other layout fields stay zero.
	n := newS7ParserNode(t, map[string]any{
		"blockLength": 16,
		"layout": []any{
			map[string]any{"offset": 0, "name": "a", "type": "real"},
			map[string]any{"offset": 4, "name": "b", "type": "dint"},
			map[string]any{"offset": 8, "name": "c", "type": "int"},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	buf, _, err := n.encode(map[string]any{"b": int64(1234567)})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(buf) != 16 {
		t.Errorf("buf length: got %d, want 16 (blockLength)", len(buf))
	}
	// Bytes 0..3 should be zero (sparse), 4..7 hold the encoded DINT, 8..15 zero.
	for i := 0; i < 4; i++ {
		if buf[i] != 0 {
			t.Errorf("byte %d: got 0x%02x, want 0 (sparse)", i, buf[i])
		}
	}
	if buf[4] != 0x00 || buf[5] != 0x12 || buf[6] != 0xD6 || buf[7] != 0x87 {
		t.Errorf("DINT 1234567: got [%02x %02x %02x %02x], want [00 12 D6 87]", buf[4], buf[5], buf[6], buf[7])
	}
	for i := 8; i < 16; i++ {
		if buf[i] != 0 {
			t.Errorf("byte %d: got 0x%02x, want 0 (sparse)", i, buf[i])
		}
	}
}

func TestS7Parser_Encode_BoolBitsORAggregated(t *testing.T) {
	// Multiple BOOLs at byte 12 — encode should pack into one byte via OR.
	n := newS7ParserNode(t, map[string]any{
		"blockLength": 16,
		"layout": []any{
			map[string]any{"offset": "12.0", "name": "alarm", "type": "bool"},
			map[string]any{"offset": "12.1", "name": "running", "type": "bool"},
			map[string]any{"offset": "12.7", "name": "maintenance", "type": "bool"},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	buf, _, err := n.encode(map[string]any{
		"alarm":       true,
		"running":     true,
		"maintenance": false,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Only bits 0 and 1 should be set in byte 12: 0b00000011 = 0x03.
	if buf[12] != 0x03 {
		t.Errorf("byte 12: got 0x%02x, want 0x03 (bits 0,1 set; bit 7 clear)", buf[12])
	}
}

func TestS7Parser_Encode_StringRoundTrip(t *testing.T) {
	n := newS7ParserNode(t, map[string]any{
		"layout": []any{
			map[string]any{"offset": 0, "name": "tag", "type": "string", "length": 20},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	buf, _, err := n.encode(map[string]any{"tag": "LOOPZE-S7-DEMO"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(buf) != 22 { // length+2 header
		t.Fatalf("buf length: got %d, want 22", len(buf))
	}
	if buf[0] != 20 || buf[1] != 14 {
		t.Errorf("STRING header: got (%d, %d), want (20, 14)", buf[0], buf[1])
	}
	if string(buf[2:16]) != "LOOPZE-S7-DEMO" {
		t.Errorf("STRING chars: got %q", string(buf[2:16]))
	}
	// Round-trip: decode and verify we get the original back.
	got, err := n.parse(buf)
	if err != nil {
		t.Fatalf("parse round-trip: %v", err)
	}
	if got["tag"] != "LOOPZE-S7-DEMO" {
		t.Errorf("round-trip tag: got %q, want %q", got["tag"], "LOOPZE-S7-DEMO")
	}
}

func TestS7Parser_Encode_RawLengthMismatch(t *testing.T) {
	n := newS7ParserNode(t, map[string]any{
		"layout": []any{
			map[string]any{"offset": 0, "name": "blob", "type": "raw", "length": 4},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_, _, err := n.encode(map[string]any{"blob": []byte{0x01, 0x02}})
	if err == nil || !strings.Contains(err.Error(), "expects 4 bytes") {
		t.Errorf("expected length-mismatch error, got %v", err)
	}
}

func TestS7Parser_Encode_AppliesInverseScale(t *testing.T) {
	// scale=0.1, valueOffset=5: encoded INT = (input - 5) / 0.1
	// For input 25.0 → INT 200 → bytes 0x00, 0xC8.
	n := newS7ParserNode(t, map[string]any{
		"layout": []any{
			map[string]any{"offset": 0, "name": "x", "type": "int", "scale": 0.1, "valueOffset": 5.0},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	buf, _, err := n.encode(map[string]any{"x": 25.0})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if buf[0] != 0x00 || buf[1] != 0xC8 {
		t.Errorf("inverse-scaled INT: got [%02x %02x], want [00 C8]", buf[0], buf[1])
	}
}

func TestS7Parser_Encode_EmptyInputErrors(t *testing.T) {
	n := newS7ParserNode(t, map[string]any{
		"layout": []any{
			map[string]any{"offset": 0, "name": "x", "type": "byte"},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_, _, err := n.encode(map[string]any{})
	if err == nil {
		t.Error("encode with empty input should error")
	}
}

// ─── Round-trip tests (parse → encode → parse) ────────────────────────────

func TestS7Parser_RoundTrip_AllNumericTypes(t *testing.T) {
	n := newS7ParserNode(t, map[string]any{
		"blockLength": 32,
		"layout": []any{
			map[string]any{"offset": 0, "name": "b", "type": "byte"},
			map[string]any{"offset": 1, "name": "w", "type": "word"},
			map[string]any{"offset": 4, "name": "i", "type": "int"},
			map[string]any{"offset": 8, "name": "d", "type": "dword"},
			map[string]any{"offset": 12, "name": "di", "type": "dint"},
			map[string]any{"offset": 16, "name": "r", "type": "real"},
		},
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	in := map[string]any{
		"b":  uint8(0x42),
		"w":  uint16(0xBEEF),
		"i":  int16(-1234),
		"d":  uint32(0xDEADBEEF),
		"di": int32(-2_000_000),
		"r":  float32(3.14),
	}
	buf, _, err := n.encode(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := n.parse(buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Errorf("round-trip mismatch:\n  got:  %#v\n  want: %#v", out, in)
	}
}

// ─── HandleMessage / dispatch ──────────────────────────────────────────────

func TestS7Parser_HandleMessage_AutoParseOnByteSlice(t *testing.T) {
	h := newS7ParserHarness(t, map[string]any{
		"layout": []any{
			map[string]any{"offset": 0, "name": "v", "type": "real"},
		},
	})
	in := flow.NewMessage()
	// REAL(1.0) = 0x3F800000
	in.SetPayload([]byte{0x3F, 0x80, 0x00, 0x00})
	out, err := h.handle(in)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	payload, ok := out.Payload().(map[string]any)
	if !ok {
		t.Fatalf("payload type: got %T, want map[string]any", out.Payload())
	}
	if payload["v"] != float32(1.0) {
		t.Errorf("v: got %v, want 1.0", payload["v"])
	}
}

func TestS7Parser_HandleMessage_AutoEncodeOnMap(t *testing.T) {
	h := newS7ParserHarness(t, map[string]any{
		"layout": []any{
			map[string]any{"offset": 0, "name": "v", "type": "real"},
		},
	})
	in := flow.NewMessage()
	in.SetPayload(map[string]any{"v": float32(1.0)})
	out, err := h.handle(in)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	buf, ok := out.Payload().([]int)
	if !ok {
		t.Fatalf("payload type: got %T, want []int (JSON-friendly byte representation)", out.Payload())
	}
	if len(buf) != 4 || buf[0] != 0x3F || buf[1] != 0x80 {
		t.Errorf("encoded REAL(1.0): got %v, want [3F 80 00 00]", buf)
	}
}

func TestS7Parser_HandleMessage_EncodeDoesNotTouchStart(t *testing.T) {
	// The parser does NOT set msg.s7.start on encode — that earlier
	// "convenience" feature put the buffer at PLC byte minOffset+layoutOffset
	// instead of layoutOffset (the layout's full-size buffer doesn't start at
	// minOffset, it covers 0..maxEnd). Users must configure `start` on the
	// downstream s7-write block explicitly.
	h := newS7ParserHarness(t, map[string]any{
		"action": "encode",
		"layout": []any{
			map[string]any{"offset": 50, "name": "tag", "type": "string", "length": 20},
		},
	})
	in := flow.NewMessage()
	in.SetPayload(map[string]any{"tag": "X"})
	out, err := h.handle(in)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if meta, ok := out.Get("s7").(map[string]any); ok {
		if _, hasStart := meta["start"]; hasStart {
			t.Errorf("msg.s7.start should NOT be set by encode; got %v", meta["start"])
		}
	}
}

func TestS7Parser_HandleMessage_EncodePreservesUserSetS7(t *testing.T) {
	// If the user set msg.s7 (e.g. area, db, start) upstream — say, via a
	// Change node — the parser must not touch it.
	h := newS7ParserHarness(t, map[string]any{
		"action": "encode",
		"layout": []any{
			map[string]any{"offset": 50, "name": "tag", "type": "string", "length": 20},
		},
	})
	in := flow.NewMessage()
	in.Set("s7", map[string]any{"start": 999, "area": "DB", "db": 1})
	in.SetPayload(map[string]any{"tag": "X"})
	out, err := h.handle(in)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	meta, _ := out.Get("s7").(map[string]any)
	if meta["start"] != 999 {
		t.Errorf("msg.s7.start should be preserved; got %v", meta["start"])
	}
	if meta["area"] != "DB" || meta["db"] != 1 {
		t.Errorf("user-set msg.s7 fields garbled: %+v", meta)
	}
}

func TestS7Parser_HandleMessage_ParseForcedFailsOnMap(t *testing.T) {
	h := newS7ParserHarness(t, map[string]any{
		"action": "parse",
		"layout": []any{
			map[string]any{"offset": 0, "name": "v", "type": "byte"},
		},
	})
	in := flow.NewMessage()
	in.SetPayload(map[string]any{"v": 1})
	_, err := h.handle(in)
	if err == nil {
		t.Error("parse on map input should error")
	}
}

func TestS7Parser_HandleMessage_PreserveBytes(t *testing.T) {
	h := newS7ParserHarness(t, map[string]any{
		"preserveBytes": true,
		"layout": []any{
			map[string]any{"offset": 0, "name": "v", "type": "byte"},
		},
	})
	in := flow.NewMessage()
	in.SetPayload([]byte{0x42})
	out, err := h.handle(in)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got, _ := out.Get("bytes").([]int); len(got) != 1 || got[0] != 0x42 {
		t.Errorf("bytes: got %v, want [0x42]", got)
	}
}

func TestS7Parser_TypeInfo(t *testing.T) {
	info := S7ParserTypeInfo()
	if info.Type != "s7-parser" {
		t.Errorf("Type: got %q, want s7-parser", info.Type)
	}
	if info.Inputs != 1 || info.Outputs != 1 {
		t.Errorf("ports: got %d in / %d out, want 1 in / 1 out", info.Inputs, info.Outputs)
	}
}

// ─── Helpers ───────────────────────────────────────────────────────────────

func newS7ParserNode(t *testing.T, props map[string]any) *S7ParserNode {
	t.Helper()
	cfg := flow.NodeConfig{
		ID: t.Name(), Type: "s7-parser", FlowID: "f1", Properties: props,
	}
	inst, err := NewS7ParserNode(cfg)
	if err != nil {
		t.Fatalf("NewS7ParserNode: %v", err)
	}
	return inst.(*S7ParserNode)
}

// s7ParserHarness wires up a parser node ready to receive HandleMessage
// calls without the engine.
type s7ParserHarness struct {
	t    *testing.T
	node *S7ParserNode
}

func newS7ParserHarness(t *testing.T, props map[string]any) *s7ParserHarness {
	t.Helper()
	n := newS7ParserNode(t, props)
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	n.SetSend(func(_ int, _ *flow.Message) {})
	n.SetStatus(func(_, _ string) {})
	n.SetDebug(noopDebug)
	n.SetError(func(_ error, _ *flow.Message) {})
	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })
	return &s7ParserHarness{t: t, node: n}
}

func (h *s7ParserHarness) handle(msg *flow.Message) (*flow.Message, error) {
	out, err := h.node.HandleMessage(msg)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 || len(out[0]) == 0 {
		return nil, nil
	}
	return out[0][0], nil
}
