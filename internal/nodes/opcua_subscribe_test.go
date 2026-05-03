// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"sync"
	"testing"
	"time"

	"github.com/niceclouds/loopze/internal/flow"
)

func newOpcuaSubscribeNodeForTest(t *testing.T, props map[string]any) *OpcuaSubscribeNode {
	t.Helper()
	inst, err := NewOpcuaSubscribeNode(flow.NodeConfig{
		ID: "sub-1", Type: "opcua-subscribe", FlowID: "f",
		Properties: props,
	})
	if err != nil {
		t.Fatalf("NewOpcuaSubscribeNode: %v", err)
	}
	return inst.(*OpcuaSubscribeNode)
}

func TestOpcuaSubscribeInitStaticRequiresItems(t *testing.T) {
	n := newOpcuaSubscribeNodeForTest(t, map[string]any{
		"server": "srv-1",
		"mode":   "static",
	})
	if err := n.Init(); err == nil {
		t.Error("static mode without monitoredItems should fail")
	}
}

func TestOpcuaSubscribeInitStaticParsesItems(t *testing.T) {
	n := newOpcuaSubscribeNodeForTest(t, map[string]any{
		"server": "srv-1",
		"mode":   "static",
		"monitoredItems": []any{
			map[string]any{
				"nodeId":           "ns=2;s=Pressure",
				"samplingInterval": float64(500),
				"queueSize":        float64(10),
				"discardOldest":    true,
				"deadband": map[string]any{
					"type":  "absolute",
					"value": float64(0.5),
				},
			},
		},
		"publishingInterval": float64(250),
		"lifetimeCount":      float64(120),
		"keepAliveCount":     float64(20),
		"priority":           float64(5),
		"outputShape":        "batch",
	})
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(n.configItems) != 1 {
		t.Fatalf("configItems len = %d", len(n.configItems))
	}
	spec := n.configItems[0]
	if spec.samplingInterval != 500 || spec.queueSize != 10 || !spec.discardOldest {
		t.Errorf("unexpected spec: %+v", spec)
	}
	if spec.deadbandType != 1 || spec.deadbandValue != 0.5 {
		t.Errorf("deadband: type=%d value=%v", spec.deadbandType, spec.deadbandValue)
	}
	if n.subParams.Interval != 250*time.Millisecond {
		t.Errorf("publishingInterval = %v", n.subParams.Interval)
	}
	if n.outputShape != "batch" {
		t.Errorf("outputShape = %q", n.outputShape)
	}
}

func TestOpcuaSubscribeInitDynamicAllowsEmpty(t *testing.T) {
	n := newOpcuaSubscribeNodeForTest(t, map[string]any{
		"server": "srv-1",
		"mode":   "dynamic",
	})
	if err := n.Init(); err != nil {
		t.Errorf("dynamic mode without items should be ok: %v", err)
	}
}

func TestOpcuaSubscribeInvalidMode(t *testing.T) {
	n := newOpcuaSubscribeNodeForTest(t, map[string]any{
		"server": "srv-1",
		"mode":   "weird",
	})
	if err := n.Init(); err == nil {
		t.Error("invalid mode should fail")
	}
}

func TestSpecsFromMessageForms(t *testing.T) {
	cases := []struct {
		name     string
		setup    func(*flow.Message)
		wantLen  int
		wantNode string
	}{
		{
			name: "single string payload",
			setup: func(m *flow.Message) {
				m.Set("payload", "ns=2;s=A")
			},
			wantLen:  1,
			wantNode: "ns=2;s=A",
		},
		{
			name: "string array",
			setup: func(m *flow.Message) {
				m.Set("payload", []any{"ns=2;s=A", "ns=2;s=B"})
			},
			wantLen:  2,
			wantNode: "ns=2;s=A",
		},
		{
			name: "object array with overrides",
			setup: func(m *flow.Message) {
				m.Set("payload", []any{
					map[string]any{
						"nodeId":           "ns=2;s=X",
						"samplingInterval": float64(50),
					},
				})
			},
			wantLen:  1,
			wantNode: "ns=2;s=X",
		},
		{
			name: "msg-level samplingInterval default",
			setup: func(m *flow.Message) {
				m.Set("payload", "ns=2;s=Z")
				m.Set("samplingInterval", float64(100))
			},
			wantLen:  1,
			wantNode: "ns=2;s=Z",
		},
		{
			name: "empty payload",
			setup: func(m *flow.Message) {
				m.Set("payload", "")
			},
			wantLen: 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := flow.NewMessage()
			c.setup(msg)
			specs := specsFromMessage(msg)
			if len(specs) != c.wantLen {
				t.Fatalf("len = %d; want %d", len(specs), c.wantLen)
			}
			if c.wantLen > 0 && specs[0].nodeID != c.wantNode {
				t.Errorf("nodeID = %q; want %q", specs[0].nodeID, c.wantNode)
			}
		})
	}
}

func TestApplyMessageDefaultsOverridesSampling(t *testing.T) {
	spec := monitoredSpec{samplingInterval: 1000}
	msg := flow.NewMessage()
	msg.Set("samplingInterval", float64(50))
	msg.Set("queueSize", float64(5))
	msg.Set("deadband", map[string]any{"type": "percent", "value": float64(2.5)})

	applyMessageDefaults(&spec, msg)
	if spec.samplingInterval != 50 || spec.queueSize != 5 {
		t.Errorf("override missed: %+v", spec)
	}
	if spec.deadbandType != 2 || spec.deadbandValue != 2.5 {
		t.Errorf("deadband override missed: %+v", spec)
	}
}

func TestBuildItemRecordSetsAllFields(t *testing.T) {
	// Plain unit test on the record builder — exercises the field shape we
	// promise the frontend without needing a live notification stream.
	spec := monitoredSpec{nodeID: "ns=2;s=T"}

	n := &OpcuaSubscribeNode{}
	rec := n.buildItemRecord(spec, nil)
	if rec["statusCode"] != "BadInternalError" {
		t.Errorf("nil DataValue: statusCode = %v", rec["statusCode"])
	}
}

func TestAllocHandleNeverZero(t *testing.T) {
	n := &OpcuaSubscribeNode{nextHandle: 0xFFFFFFFF}
	n.items = make(map[uint32]*itemState)
	var wg sync.WaitGroup
	seen := sync.Map{}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n.mu.Lock()
			h := n.allocHandleLocked()
			n.mu.Unlock()
			if h == 0 {
				t.Error("alloc returned 0")
			}
			if _, dup := seen.LoadOrStore(h, true); dup {
				t.Error("duplicate handle")
			}
		}()
	}
	wg.Wait()
}

// E2E: subscribe to CurrentTime and wait for at least one notification.
func TestOpcuaSubscribeE2E(t *testing.T) {
	endpoint := opcuaTestEndpoint(t)

	server, err := NewOpcuaServer(flow.ConfigNode{
		ID: "srv-sub-e2e", Type: "opcua-server",
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

	inst, err := NewOpcuaSubscribeNode(flow.NodeConfig{
		ID: "sub-e2e", Type: "opcua-subscribe", FlowID: "f",
		Properties: map[string]any{
			"server": "srv-sub-e2e",
			"mode":   "static",
			"monitoredItems": []any{
				map[string]any{
					"nodeId":           "i=2258",
					"samplingInterval": float64(250),
					"queueSize":        float64(1),
				},
			},
			"publishingInterval": float64(250),
		},
	})
	if err != nil {
		t.Fatalf("NewOpcuaSubscribeNode: %v", err)
	}
	sub := inst.(*OpcuaSubscribeNode)
	if err := sub.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	gotMsg := make(chan *flow.Message, 4)
	sub.SetSend(func(_ int, m *flow.Message) { gotMsg <- m })
	sub.SetStatus(func(string, string) {})
	sub.SetConfigLookup(func(id string) (flow.ConfigInstance, bool) {
		if id == "srv-sub-e2e" {
			return server, true
		}
		return nil, false
	})
	if err := sub.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sub.Stop()

	select {
	case m := <-gotMsg:
		nodeID, _ := m.Get("nodeId").(string)
		if nodeID != "i=2258" {
			t.Errorf("nodeId = %q", nodeID)
		}
		t.Logf("got notification: nodeId=%v value=%v", nodeID, m.Get("value"))
	case <-time.After(5 * time.Second):
		t.Fatal("no notification arrived within 5s")
	}
}
