// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"context"
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

func TestNodeClassName(t *testing.T) {
	cases := map[ua.NodeClass]string{
		ua.NodeClassObject:        "Object",
		ua.NodeClassVariable:      "Variable",
		ua.NodeClassMethod:        "Method",
		ua.NodeClassDataType:      "DataType",
		ua.NodeClassUnspecified:   "",
	}
	for nc, want := range cases {
		if got := nodeClassName(nc); got != want {
			t.Errorf("nodeClassName(%v) = %q; want %q", nc, got, want)
		}
	}
}

func TestAccessLevelName(t *testing.T) {
	cases := []struct {
		bits byte
		want string
	}{
		{0, "None"},
		{0x01, "Read"},
		{0x03, "Read|Write"},
		{0x07, "Read|Write|HistoryRead"},
	}
	for _, c := range cases {
		if got := accessLevelName(c.bits); got != c.want {
			t.Errorf("accessLevelName(0x%X) = %q; want %q", c.bits, got, c.want)
		}
	}
}

func TestOpcuaBrowseAdHocE2E(t *testing.T) {
	endpoint := opcuaTestEndpoint(t)

	cfg := flow.ConfigNode{
		ID:     "browse-e2e",
		Type:   "opcua-server",
		Config: map[string]any{"endpointUrl": endpoint},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Browse the root Objects folder. Every conformant server exposes children
	// here; we don't assert on specific names since the test server is bring-
	// your-own.
	res, err := OpcuaBrowseAdHoc(ctx, cfg, "")
	if err != nil {
		t.Fatalf("Browse Objects: %v", err)
	}
	if len(res.Children) == 0 {
		t.Error("Objects folder should expose children")
	}
	if res.Parent == nil || res.Parent.NodeID != "i=85" {
		t.Errorf("parent NodeID = %v", res.Parent)
	}
	t.Logf("Objects folder: %d children, first = %+v", len(res.Children), res.Children[0])
}
