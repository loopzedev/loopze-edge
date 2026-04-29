// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"reflect"
	"testing"

	"github.com/gopcua/opcua/ua"
)

// motorStatusDef is a synthetic schema we use across the codec tests. It
// covers the operational mix industrial servers typically expose: scalars
// of different built-in types, an array, an enum and a nested struct.
func motorStatusDef() *StructDef {
	limits := &StructDef{
		Name:          "Limits",
		StructureType: ua.StructureTypeStructure,
		Fields: []StructField{
			{Name: "Min", BuiltinType: ua.TypeIDDouble},
			{Name: "Max", BuiltinType: ua.TypeIDDouble},
		},
	}
	mode := &EnumDef{
		Name: "MotorMode",
		ByValue: map[int64]string{
			0: "Off",
			1: "Manual",
			2: "Auto",
		},
		ByName: map[string]int64{
			"Off":    0,
			"Manual": 1,
			"Auto":   2,
		},
	}
	return &StructDef{
		Name:          "MotorStatus",
		StructureType: ua.StructureTypeStructure,
		Fields: []StructField{
			{Name: "Speed", BuiltinType: ua.TypeIDDouble},
			{Name: "FaultCode", BuiltinType: ua.TypeIDInt32},
			{Name: "Running", BuiltinType: ua.TypeIDBoolean},
			{Name: "Mode", BuiltinType: ua.TypeIDInt32, EnumDef: mode},
			{Name: "Tags", IsArray: true, BuiltinType: ua.TypeIDString},
			{Name: "Limits", NestedDef: limits},
		},
	}
}

func TestExtObjEncodeDecodeRoundTrip(t *testing.T) {
	def := motorStatusDef()

	in := map[string]any{
		"Speed":     float64(1450.5),
		"FaultCode": int32(0),
		"Running":   true,
		"Mode":      "Auto",
		"Tags":      []any{"hot", "spinning"},
		"Limits":    map[string]any{"Min": float64(0), "Max": float64(3000)},
	}

	body, err := EncodeStructBinary(in, def)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("encoded body is empty")
	}

	out, err := DecodeStructBinary(body, def)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if out["Speed"].(float64) != 1450.5 {
		t.Errorf("Speed = %v", out["Speed"])
	}
	if out["FaultCode"].(int32) != 0 {
		t.Errorf("FaultCode = %v", out["FaultCode"])
	}
	if out["Running"].(bool) != true {
		t.Errorf("Running = %v", out["Running"])
	}

	mode, ok := out["Mode"].(map[string]any)
	if !ok {
		t.Fatalf("Mode kind: %T", out["Mode"])
	}
	if mode["name"] != "Auto" || mode["value"].(int64) != 2 {
		t.Errorf("Mode mapping: %+v", mode)
	}

	tags := out["Tags"].([]any)
	if len(tags) != 2 || tags[0] != "hot" || tags[1] != "spinning" {
		t.Errorf("Tags: %+v", tags)
	}

	limits := out["Limits"].(map[string]any)
	if limits["Min"].(float64) != 0 || limits["Max"].(float64) != 3000 {
		t.Errorf("Limits: %+v", limits)
	}
}

func TestExtObjMissingFieldsGetDefaults(t *testing.T) {
	def := motorStatusDef()

	body, err := EncodeStructBinary(map[string]any{
		"Speed": float64(800),
		// Everything else missing → encoder fills zero values.
	}, def)
	if err != nil {
		t.Fatalf("Encode with missing fields: %v", err)
	}
	out, err := DecodeStructBinary(body, def)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out["Running"].(bool) != false || out["FaultCode"].(int32) != 0 {
		t.Errorf("defaults not applied: %+v", out)
	}
}

func TestExtObjExtraFieldsIgnored(t *testing.T) {
	// Schema only knows two fields; we throw three at it.
	def := &StructDef{
		StructureType: ua.StructureTypeStructure,
		Fields: []StructField{
			{Name: "A", BuiltinType: ua.TypeIDInt32},
			{Name: "B", BuiltinType: ua.TypeIDInt32},
		},
	}
	body, err := EncodeStructBinary(map[string]any{
		"A":     float64(1),
		"B":     float64(2),
		"Extra": "ignore me",
	}, def)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out, err := DecodeStructBinary(body, def)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out["A"].(int32) != 1 || out["B"].(int32) != 2 {
		t.Errorf("decoded: %+v", out)
	}
	if _, hasExtra := out["Extra"]; hasExtra {
		t.Error("Extra should not appear in decoded output")
	}
}

func TestExtObjTypeMismatchReturnsError(t *testing.T) {
	def := &StructDef{
		StructureType: ua.StructureTypeStructure,
		Fields: []StructField{
			{Name: "X", BuiltinType: ua.TypeIDInt32},
		},
	}
	_, err := EncodeStructBinary(map[string]any{"X": "definitely not a number"}, def)
	if err == nil {
		t.Error("expected error for string-into-int32")
	}
}

func TestExtObjEnumAcceptsBothFormsOnEncode(t *testing.T) {
	def := &StructDef{
		StructureType: ua.StructureTypeStructure,
		Fields: []StructField{
			{
				Name:        "Mode",
				BuiltinType: ua.TypeIDInt32,
				EnumDef: &EnumDef{
					ByValue: map[int64]string{0: "Off", 1: "On"},
					ByName:  map[string]int64{"Off": 0, "On": 1},
				},
			},
		},
	}
	for _, in := range []any{
		"On",
		map[string]any{"name": "On"},
		float64(1),
	} {
		body, err := EncodeStructBinary(map[string]any{"Mode": in}, def)
		if err != nil {
			t.Errorf("Encode(%v): %v", in, err)
			continue
		}
		out, err := DecodeStructBinary(body, def)
		if err != nil {
			t.Errorf("Decode after %v: %v", in, err)
			continue
		}
		mode := out["Mode"].(map[string]any)
		if mode["value"].(int64) != 1 {
			t.Errorf("Encode(%v) produced value %v", in, mode["value"])
		}
	}
}

func TestExtObjOptionalFieldsRoundTrip(t *testing.T) {
	def := &StructDef{
		StructureType: ua.StructureTypeStructureWithOptionalFields,
		Fields: []StructField{
			{Name: "A", BuiltinType: ua.TypeIDInt32},
			{Name: "B", BuiltinType: ua.TypeIDInt32, IsOptional: true},
			{Name: "C", BuiltinType: ua.TypeIDString, IsOptional: true},
		},
	}
	OptionalFieldCount := 2
	def.OptionalFieldCount = OptionalFieldCount

	in := map[string]any{
		"A": float64(7),
		// B intentionally absent → bit 0 = 0
		"C": "present",
	}
	body, err := EncodeStructBinary(in, def)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out, err := DecodeStructBinary(body, def)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if _, hasB := out["B"]; hasB {
		t.Error("B should be absent in decoded output")
	}
	if out["A"].(int32) != 7 || out["C"].(string) != "present" {
		t.Errorf("decoded: %+v", out)
	}
}

func TestExtObjUnionRoundTrip(t *testing.T) {
	def := &StructDef{
		StructureType: ua.StructureTypeUnion,
		Fields: []StructField{
			{Name: "AsInt", BuiltinType: ua.TypeIDInt32},
			{Name: "AsString", BuiltinType: ua.TypeIDString},
		},
	}
	body, err := EncodeStructBinary(map[string]any{"AsString": "hello"}, def)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out, err := DecodeStructBinary(body, def)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(out, map[string]any{"AsString": "hello"}) {
		t.Errorf("decoded: %+v", out)
	}
}

func TestRegisterRawExtObjTypeIdempotent(t *testing.T) {
	// Two calls with the same NodeID must not panic and must not register
	// twice from gopcua's perspective.
	id := ua.MustParseNodeID("ns=42;i=1")
	registerRawExtObjType(id)
	registerRawExtObjType(id)
}
