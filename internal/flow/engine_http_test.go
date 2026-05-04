// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow_test

import (
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/config"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// httpInStub is a minimal node that implements flow.HTTPInProvider and
// flow.HTTPMuxProvider. It records what the engine injected so the test
// can assert on the wiring.
type httpInStub struct {
	method string
	path   string

	mu       sync.Mutex
	registry *flow.ResponseRegistry
	root     string
}

func (n *httpInStub) Init() error                                              { return nil }
func (n *httpInStub) SetSend(_ flow.SendFunc)                                  {}
func (n *httpInStub) SetStatus(_ flow.StatusFunc)                              {}
func (n *httpInStub) SetDebug(_ flow.DebugFunc)                                {}
func (n *httpInStub) Start() error                                             { return nil }
func (n *httpInStub) Stop() error                                              { return nil }
func (n *httpInStub) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) { return nil, nil }

func (n *httpInStub) SetHTTPMux(registry *flow.ResponseRegistry, root string) {
	n.mu.Lock()
	n.registry = registry
	n.root = root
	n.mu.Unlock()
}

func (n *httpInStub) HTTPRoute() flow.HTTPRouteSpec {
	return flow.HTTPRouteSpec{
		Method:  n.method,
		Path:    n.path,
		Handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
	}
}

func TestEngineCallsHTTPMuxBuilderOnDeploy(t *testing.T) {
	cfg := &config.Config{}
	engine := flow.NewEngine(cfg)

	var (
		buildCount   atomic.Int32
		gotSpecsCopy []flow.HTTPRouteSpec
		specsMu      sync.Mutex
	)
	engine.SetHTTPMuxBuilder(func(specs []flow.HTTPRouteSpec) []flow.HTTPRouteConflict {
		buildCount.Add(1)
		specsMu.Lock()
		gotSpecsCopy = append([]flow.HTTPRouteSpec(nil), specs...)
		specsMu.Unlock()
		return nil
	}, "/endpoint")

	engine.Registry().Register("http-in-stub", func(c flow.NodeConfig) (flow.NodeInstance, error) {
		return &httpInStub{method: "POST", path: "/webhook"}, nil
	}, flow.NodeTypeInfo{Type: "http-in-stub", Inputs: 0, Outputs: 1})

	if err := engine.Start(); err != nil {
		t.Fatal(err)
	}
	defer engine.Stop()

	flows := []flow.Flow{{
		ID:    "f1",
		Type:  "tab",
		Nodes: []flow.Node{{ID: "n1", Type: "http-in-stub", Z: "f1", Inputs: 0, Outputs: 1}},
	}}
	if err := engine.Deploy(flows, nil, flow.DeployFull); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	if buildCount.Load() < 1 {
		t.Fatalf("builder was not called on deploy (count=%d)", buildCount.Load())
	}

	specsMu.Lock()
	got := gotSpecsCopy
	specsMu.Unlock()
	if len(got) != 1 {
		t.Fatalf("specs collected: got %d, want 1", len(got))
	}
	if got[0].NodeID != "n1" || got[0].Method != "POST" || got[0].Path != "/webhook" {
		t.Errorf("spec mismatch: got %+v", got[0])
	}
}

func TestEngineInjectsRegistryAndRoot(t *testing.T) {
	cfg := &config.Config{}
	engine := flow.NewEngine(cfg)
	engine.SetHTTPMuxBuilder(func(_ []flow.HTTPRouteSpec) []flow.HTTPRouteConflict { return nil }, "/hooks")

	stub := &httpInStub{method: "GET", path: "/x"}
	engine.Registry().Register("http-in-stub", func(_ flow.NodeConfig) (flow.NodeInstance, error) {
		return stub, nil
	}, flow.NodeTypeInfo{Type: "http-in-stub", Inputs: 0, Outputs: 1})

	if err := engine.Start(); err != nil {
		t.Fatal(err)
	}
	defer engine.Stop()

	flows := []flow.Flow{{
		ID:    "f1",
		Type:  "tab",
		Nodes: []flow.Node{{ID: "n1", Type: "http-in-stub", Z: "f1", Inputs: 0, Outputs: 1}},
	}}
	if err := engine.Deploy(flows, nil, flow.DeployFull); err != nil {
		t.Fatal(err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.registry == nil {
		t.Error("registry was not injected")
	}
	if stub.registry != engine.ResponseRegistry() {
		t.Error("injected registry differs from engine.ResponseRegistry()")
	}
	if stub.root != "/hooks" {
		t.Errorf("injected root: got %q, want %q", stub.root, "/hooks")
	}
}

func TestEngineForwardsConflictsToErrorFunc(t *testing.T) {
	cfg := &config.Config{}
	engine := flow.NewEngine(cfg)

	// Builder reports a synthetic conflict for node n1.
	engine.SetHTTPMuxBuilder(func(specs []flow.HTTPRouteSpec) []flow.HTTPRouteConflict {
		var out []flow.HTTPRouteConflict
		for _, sp := range specs {
			out = append(out, flow.HTTPRouteConflict{
				NodeID: sp.NodeID,
				Method: sp.Method,
				Path:   sp.Path,
				Reason: "forced for test",
			})
		}
		return out
	}, "/endpoint")

	engine.Registry().Register("http-in-stub", func(_ flow.NodeConfig) (flow.NodeInstance, error) {
		return &httpInStub{method: "GET", path: "/x"}, nil
	}, flow.NodeTypeInfo{Type: "http-in-stub", Inputs: 0, Outputs: 1})

	// Catch the error message routed via errorFn → publishNodeError →
	// PublishDebug. Capture into a slice for assertion.
	var (
		mu      sync.Mutex
		debugs  []flow.DebugMessage
	)
	engine.SetPublishDebug(func(_ string, msg flow.DebugMessage) {
		mu.Lock()
		defer mu.Unlock()
		debugs = append(debugs, msg)
	})

	if err := engine.Start(); err != nil {
		t.Fatal(err)
	}
	defer engine.Stop()

	flows := []flow.Flow{{
		ID:    "f1",
		Type:  "tab",
		Nodes: []flow.Node{{ID: "n1", Type: "http-in-stub", Z: "f1", Inputs: 0, Outputs: 1}},
	}}
	if err := engine.Deploy(flows, nil, flow.DeployFull); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, d := range debugs {
		if d.NodeID == "n1" && d.Status == "error" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected an error debug message for node n1, got %+v", debugs)
	}
}

func TestEngineStopDrainsResponseRegistry(t *testing.T) {
	cfg := &config.Config{}
	engine := flow.NewEngine(cfg)

	if err := engine.Start(); err != nil {
		t.Fatal(err)
	}

	// Manually register a slot to simulate an in-flight request.
	registry := engine.ResponseRegistry()
	rec := newDoneRecorder()
	_, done := registry.Register(rec, nil, 0, "n1", "f1")

	if registry.Len() != 1 {
		t.Fatalf("pre-stop len: %d, want 1", registry.Len())
	}

	if err := engine.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	select {
	case <-done:
	default:
		t.Fatal("Stop() did not drain the response registry")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("drain response: got %d, want 503", rec.Code)
	}
}

// doneRecorder is a stripped-down http.ResponseWriter so the registry
// can write into something we control without pulling httptest into a
// _test.go that already imports a lot.
type doneRecorder struct {
	header http.Header
	Code   int
	body   []byte
}

func newDoneRecorder() *doneRecorder {
	return &doneRecorder{header: make(http.Header), Code: http.StatusOK}
}

func (r *doneRecorder) Header() http.Header        { return r.header }
func (r *doneRecorder) Write(p []byte) (int, error) { r.body = append(r.body, p...); return len(p), nil }
func (r *doneRecorder) WriteHeader(code int)        { r.Code = code }
