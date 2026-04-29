// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
)

func TestCoerceOpcuaValueIntegers(t *testing.T) {
	cases := []struct {
		val    any
		target ua.TypeID
		want   any
		errOK  bool
	}{
		{float64(42), ua.TypeIDInt32, int32(42), false},
		{float64(70000), ua.TypeIDInt16, int16(0), true},        // out of range
		{"42", ua.TypeIDInt32, int32(42), false},
		{"abc", ua.TypeIDInt32, int32(0), true},
		{float64(-1), ua.TypeIDByte, uint8(0), true},            // negative for unsigned
		{float64(255), ua.TypeIDByte, uint8(255), false},
		{float64(3.14), ua.TypeIDInt32, int32(0), true},         // non-integer
		{true, ua.TypeIDInt32, int32(1), false},
	}
	for _, c := range cases {
		got, err := CoerceOpcuaValue(c.val, c.target)
		if (err != nil) != c.errOK {
			t.Errorf("Coerce(%v, %v): err=%v want errOK=%v", c.val, c.target, err, c.errOK)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("Coerce(%v, %v) = %v (%T); want %v (%T)", c.val, c.target, got, got, c.want, c.want)
		}
	}
}

func TestCoerceOpcuaValueFloats(t *testing.T) {
	got, err := CoerceOpcuaValue(float64(3.14), ua.TypeIDDouble)
	if err != nil || got.(float64) != 3.14 {
		t.Errorf("Double 3.14: %v %v", got, err)
	}
	got, err = CoerceOpcuaValue("3.14", ua.TypeIDDouble)
	if err != nil || got.(float64) != 3.14 {
		t.Errorf("Double parse 3.14: %v %v", got, err)
	}
	got, err = CoerceOpcuaValue(float64(1.5), ua.TypeIDFloat)
	if err != nil || got.(float32) != 1.5 {
		t.Errorf("Float 1.5: %v %v", got, err)
	}
}

func TestCoerceOpcuaValueBoolean(t *testing.T) {
	for _, v := range []any{true, "true", "1", float64(1)} {
		got, err := CoerceOpcuaValue(v, ua.TypeIDBoolean)
		if err != nil || got.(bool) != true {
			t.Errorf("Boolean truthy %v: %v %v", v, got, err)
		}
	}
	for _, v := range []any{false, "false", "0", float64(0)} {
		got, err := CoerceOpcuaValue(v, ua.TypeIDBoolean)
		if err != nil || got.(bool) != false {
			t.Errorf("Boolean falsy %v: %v %v", v, got, err)
		}
	}
	if _, err := CoerceOpcuaValue("yes", ua.TypeIDBoolean); err == nil {
		t.Error(`"yes" should not coerce to Boolean`)
	}
}

func TestCoerceOpcuaValueString(t *testing.T) {
	got, err := CoerceOpcuaValue(float64(42), ua.TypeIDString)
	if err != nil || got.(string) != "42" {
		t.Errorf("String from float: %v %v", got, err)
	}
}

func TestCoerceOpcuaValueDateTime(t *testing.T) {
	got, err := CoerceOpcuaValue("2026-04-28T12:00:00Z", ua.TypeIDDateTime)
	if err != nil {
		t.Fatalf("DateTime parse: %v", err)
	}
	if got.(time.Time).Year() != 2026 {
		t.Errorf("year = %d", got.(time.Time).Year())
	}
}

func TestCoerceOpcuaValueByteString(t *testing.T) {
	got, err := CoerceOpcuaValue([]any{float64(0xDE), float64(0xAD)}, ua.TypeIDByteString)
	if err != nil {
		t.Fatalf("ByteString from []any: %v", err)
	}
	b := got.([]byte)
	if len(b) != 2 || b[0] != 0xDE || b[1] != 0xAD {
		t.Errorf("bytes = %x", b)
	}

	got, err = CoerceOpcuaValue("0xDEAD", ua.TypeIDByteString)
	if err != nil || len(got.([]byte)) != 2 {
		t.Errorf("ByteString from hex: %v %v", got, err)
	}
}

func TestCoerceOpcuaValueExtensionObjectRejected(t *testing.T) {
	if _, err := CoerceOpcuaValue(map[string]any{"foo": 1}, ua.TypeIDExtensionObject); err == nil {
		t.Error("ExtensionObject coercion should error in Phase 3 (deferred to Phase 5)")
	}
}
