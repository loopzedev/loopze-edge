// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

func TestOpcuaTypeIDFromName(t *testing.T) {
	cases := []struct {
		name    string
		want    ua.TypeID
		wantOK  bool
	}{
		{"Int32", ua.TypeIDInt32, true},
		{"Double", ua.TypeIDDouble, true},
		{"String", ua.TypeIDString, true},
		{"Boolean", ua.TypeIDBoolean, true},
		{"ExtensionObject", ua.TypeIDExtensionObject, true},
		{"Bogus", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := OpcuaTypeIDFromName(c.name)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Errorf("OpcuaTypeIDFromName(%q) = (%v,%v); want (%v,%v)", c.name, got, ok, c.want, c.wantOK)
		}
	}
}

func TestOpcuaTypeName(t *testing.T) {
	if got := OpcuaTypeName(ua.TypeIDInt32); got != "Int32" {
		t.Errorf("Int32: got %q", got)
	}
	if got := OpcuaTypeName(ua.TypeID(99)); got != "Unknown" {
		t.Errorf("unknown TypeID: got %q", got)
	}
}

func TestOpcuaAttributeIDFromName(t *testing.T) {
	id, ok := OpcuaAttributeIDFromName("DataType")
	if !ok || id != ua.AttributeIDDataType {
		t.Errorf("DataType: got (%v,%v)", id, ok)
	}
	id, ok = OpcuaAttributeIDFromName("")
	if ok || id != ua.AttributeIDValue {
		t.Errorf("empty: got (%v,%v); want (Value,false)", id, ok)
	}
}

func TestOpcuaStatusCodeName(t *testing.T) {
	if got := OpcuaStatusCodeName(ua.StatusOK); got != "Good" {
		t.Errorf("Good: got %q", got)
	}
	if got := OpcuaStatusCodeName(ua.StatusBadNodeIDUnknown); got != "BadNodeIDUnknown" {
		t.Errorf("BadNodeIDUnknown: got %q", got)
	}
	if got := OpcuaStatusCodeName(ua.StatusCode(0x80FFFFFF)); got != "Bad_0x80FFFFFF" {
		t.Errorf("unknown: got %q", got)
	}
}

func TestOpcuaStatusCodeIsGood(t *testing.T) {
	if !OpcuaStatusCodeIsGood(ua.StatusOK) {
		t.Error("StatusOK should be Good")
	}
	if OpcuaStatusCodeIsGood(ua.StatusBad) {
		t.Error("StatusBad must not be Good")
	}
	if OpcuaStatusCodeIsGood(ua.StatusUncertain) {
		t.Error("Uncertain must not be Good")
	}
}

func TestOpcuaParseAndFormatNodeID(t *testing.T) {
	cases := []string{
		"i=2258",
		"ns=2;i=42",
		"ns=2;s=Demo.Static.Scalar.Int32",
	}
	for _, s := range cases {
		id, err := ParseOpcuaNodeID(s)
		if err != nil {
			t.Errorf("Parse(%q): %v", s, err)
			continue
		}
		got := FormatOpcuaNodeID(id)
		if got == "" {
			t.Errorf("Format(%q) empty", s)
		}
	}
	if _, err := ParseOpcuaNodeID(""); err == nil {
		t.Error("empty NodeID should error")
	}
}

// TestOpcuaSpike validates the gopcua/opcua client end-to-end against an
// external test server (the Deno-based server lives in a separate project).
// Skipped unless FLINT_OPCUA_TEST_ENDPOINT is set so plain `go test ./...`
// stays green on contributor machines without the test server.
func TestOpcuaSpike(t *testing.T) {
	endpoint := os.Getenv("FLINT_OPCUA_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("set FLINT_OPCUA_TEST_ENDPOINT to run (e.g. opc.tcp://localhost:4840)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := opcua.NewClient(endpoint, opcua.SecurityMode(ua.MessageSecurityModeNone))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close(ctx) }()

	// Read CurrentTime — every conformant OPC UA server exposes it at i=2258.
	currentTime, err := ParseOpcuaNodeID("i=2258")
	if err != nil {
		t.Fatalf("ParseNodeID: %v", err)
	}
	resp, err := c.Read(ctx, &ua.ReadRequest{
		TimestampsToReturn: ua.TimestampsToReturnBoth,
		NodesToRead: []*ua.ReadValueID{
			{NodeID: currentTime, AttributeID: ua.AttributeIDValue},
		},
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	res := resp.Results[0]
	if !OpcuaStatusCodeIsGood(res.Status) {
		t.Fatalf("CurrentTime read status: %s", OpcuaStatusCodeName(res.Status))
	}
	if res.Value == nil || res.Value.Value() == nil {
		t.Fatal("CurrentTime value is nil")
	}
	t.Logf("CurrentTime read OK: %v (status=%s)", res.Value.Value(), OpcuaStatusCodeName(res.Status))
}
