// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package core

import (
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

func TestCatchNode_InitParsesScope(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want string
	}{
		{"flow", "flow", "flow"},
		{"selected", "selected", "selected"},
		{"all", "all", "all"},
		{"unknown falls back to flow", "garbage", "flow"},
		{"missing falls back to flow", nil, "flow"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			props := map[string]any{}
			if tc.raw != nil {
				props["scope"] = tc.raw
			}
			n, err := NewCatchNode(flow.NodeConfig{ID: "c", Properties: props})
			if err != nil {
				t.Fatalf("NewCatchNode: %v", err)
			}
			if err := n.Init(); err != nil {
				t.Fatalf("Init: %v", err)
			}
			cn := n.(*CatchNode)
			if cn.scope != tc.want {
				t.Errorf("scope: want %q, got %q", tc.want, cn.scope)
			}
		})
	}
}

func TestCatchNode_InitParsesTargetNodes(t *testing.T) {
	n, err := NewCatchNode(flow.NodeConfig{
		ID: "c",
		Properties: map[string]any{
			"scope":       "selected",
			"targetNodes": []any{"a", "b", "", "c"},
		},
	})
	if err != nil {
		t.Fatalf("NewCatchNode: %v", err)
	}
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cn := n.(*CatchNode)
	if len(cn.targetNodes) != 3 {
		t.Fatalf("expected 3 target nodes (empty string filtered), got %d", len(cn.targetNodes))
	}
	for _, id := range []string{"a", "b", "c"} {
		if _, ok := cn.targetNodes[id]; !ok {
			t.Errorf("target %q missing from set", id)
		}
	}
}

func TestCatchNode_HandleMessageIsNoop(t *testing.T) {
	n, _ := NewCatchNode(flow.NodeConfig{ID: "c"})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	out, err := n.HandleMessage(flow.NewMessage())
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if out != nil {
		t.Errorf("HandleMessage must return nil, got %v", out)
	}
}

func TestCatchTypeInfo_Defaults(t *testing.T) {
	info := CatchTypeInfo()
	if info.Type != "catch" {
		t.Errorf("type: %q", info.Type)
	}
	if info.Inputs != 0 || info.Outputs != 1 {
		t.Errorf("inputs/outputs: %d/%d", info.Inputs, info.Outputs)
	}
	if info.Defaults["scope"] != "flow" {
		t.Errorf("default scope: %v", info.Defaults["scope"])
	}
}
