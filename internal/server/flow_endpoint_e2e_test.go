// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/loopzedev/loopze-edge/internal/config"
	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes/network"
	"github.com/loopzedev/loopze-edge/internal/server"
)

// passthroughNode is a 1-in/1-out helper that forwards every message
// straight through. Used in e2e tests to bridge http-in → http-response.
type passthroughNode struct {
	send flow.SendFunc
}

func (n *passthroughNode) Init() error                { return nil }
func (n *passthroughNode) SetSend(fn flow.SendFunc)   { n.send = fn }
func (n *passthroughNode) SetStatus(_ flow.StatusFunc) {}
func (n *passthroughNode) SetDebug(_ flow.DebugFunc)   {}
func (n *passthroughNode) Start() error                { return nil }
func (n *passthroughNode) Stop() error                 { return nil }
func (n *passthroughNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	return [][]*flow.Message{{msg}}, nil
}

// stamperNode mutates msg.payload to a fixed string before forwarding,
// so we can assert that the http-in path actually traversed the flow.
type stamperNode struct {
	send  flow.SendFunc
	stamp string
}

func (n *stamperNode) Init() error                { return nil }
func (n *stamperNode) SetSend(fn flow.SendFunc)   { n.send = fn }
func (n *stamperNode) SetStatus(_ flow.StatusFunc) {}
func (n *stamperNode) SetDebug(_ flow.DebugFunc)   {}
func (n *stamperNode) Start() error                { return nil }
func (n *stamperNode) Stop() error                 { return nil }
func (n *stamperNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	msg.SetPayload(n.stamp)
	return [][]*flow.Message{{msg}}, nil
}

// buildE2EHarness returns an httptest.Server whose chi router mounts a
// FlowEndpointMux that the given engine has been wired into. Caller is
// responsible for engine.Stop() and tsrv.Close().
func buildE2EHarness(t *testing.T, engine *flow.Engine) (*httptest.Server, *server.FlowEndpointMux) {
	t.Helper()
	mux := server.NewFlowEndpointMux()
	engine.SetHTTPMuxBuilder(func(specs []flow.HTTPRouteSpec) []flow.HTTPRouteConflict {
		internal := make([]server.RouteSpec, len(specs))
		for i, sp := range specs {
			internal[i] = server.RouteSpec{
				NodeID:  sp.NodeID,
				Method:  sp.Method,
				Path:    sp.Path,
				Handler: sp.Handler,
			}
		}
		conflicts := mux.Swap(internal)
		out := make([]flow.HTTPRouteConflict, len(conflicts))
		for i, c := range conflicts {
			out[i] = flow.HTTPRouteConflict{
				NodeID: c.Spec.NodeID,
				Method: c.Spec.Method,
				Path:   c.Spec.Path,
				Reason: c.Reason,
			}
		}
		return out
	}, "/endpoint")

	r := chi.NewRouter()
	r.Mount("/endpoint", mux)
	return httptest.NewServer(r), mux
}

// TestEndToEndInFunctionResponse exercises the full path: real HTTP
// client → flow-endpoint mux → http-in handler → flow message →
// stamper → http-response → response back to the caller.
func TestEndToEndInFunctionResponse(t *testing.T) {
	cfg := &config.Config{HTTPNodeRoot: "/endpoint"}
	engine := flow.NewEngine(cfg)

	engine.Registry().Register("http-in", network.NewHTTPInNode, network.HTTPInTypeInfo())
	engine.Registry().Register("http-response", network.NewHTTPResponseNode, network.HTTPResponseTypeInfo())
	engine.Registry().Register("test-stamper", func(_ flow.NodeConfig) (flow.NodeInstance, error) {
		return &stamperNode{stamp: "stamped-by-flow"}, nil
	}, flow.NodeTypeInfo{Type: "test-stamper", Inputs: 1, Outputs: 1})

	tsrv, _ := buildE2EHarness(t, engine)
	defer tsrv.Close()

	if err := engine.Start(); err != nil {
		t.Fatal(err)
	}
	defer engine.Stop()

	flows := []flow.Flow{{
		ID: "f1", Type: "tab",
		Nodes: []flow.Node{
			{
				ID: "in1", Type: "http-in", Z: "f1", Inputs: 0, Outputs: 1,
				Wires:  [][]string{{"st1"}},
				Config: map[string]any{"method": "POST", "path": "/webhook", "bodyParse": "auto"},
			},
			{
				ID: "st1", Type: "test-stamper", Z: "f1", Inputs: 1, Outputs: 1,
				Wires: [][]string{{"resp1"}},
			},
			{
				ID: "resp1", Type: "http-response", Z: "f1", Inputs: 1, Outputs: 0,
				Config: map[string]any{"statusCode": 201},
			},
		},
	}}
	if err := engine.Deploy(flows, nil, flow.DeployFull); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	resp, err := http.Post(tsrv.URL+"/endpoint/webhook", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 201 {
		t.Errorf("status: got %d, want 201", resp.StatusCode)
	}
	if string(body) != "stamped-by-flow" {
		t.Errorf("body: got %q, want %q", string(body), "stamped-by-flow")
	}
}

// TestRedeployDrainsInFlightRequests verifies that an in-flight HTTP
// request started against the previous deploy is drained with 503 when
// a redeploy happens.
func TestRedeployDrainsInFlightRequests(t *testing.T) {
	cfg := &config.Config{HTTPNodeRoot: "/endpoint"}
	engine := flow.NewEngine(cfg)

	engine.Registry().Register("http-in", network.NewHTTPInNode, network.HTTPInTypeInfo())

	// A "stuck" passthrough that swallows the message — http-response
	// will never be called, so the request blocks until the registry
	// is drained.
	engine.Registry().Register("test-stuck", func(_ flow.NodeConfig) (flow.NodeInstance, error) {
		return &stuckNode{}, nil
	}, flow.NodeTypeInfo{Type: "test-stuck", Inputs: 1, Outputs: 1})

	tsrv, _ := buildE2EHarness(t, engine)
	defer tsrv.Close()

	if err := engine.Start(); err != nil {
		t.Fatal(err)
	}
	defer engine.Stop()

	v1 := []flow.Flow{{
		ID: "f1", Type: "tab",
		Nodes: []flow.Node{
			{
				ID: "in1", Type: "http-in", Z: "f1", Inputs: 0, Outputs: 1,
				Wires:  [][]string{{"stuck1"}},
				Config: map[string]any{"method": "GET", "path": "/slow"},
			},
			{
				ID: "stuck1", Type: "test-stuck", Z: "f1", Inputs: 1, Outputs: 1,
			},
		},
	}}
	if err := engine.Deploy(v1, nil, flow.DeployFull); err != nil {
		t.Fatal(err)
	}

	// Fire a request that will block in the http-in handler.
	type result struct {
		status int
		body   string
		err    error
	}
	resCh := make(chan result, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		r, err := client.Get(tsrv.URL + "/endpoint/slow")
		if err != nil {
			resCh <- result{err: err}
			return
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		resCh <- result{status: r.StatusCode, body: string(b)}
	}()

	// Wait until the slot is registered (handler reached <-done).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if engine.ResponseRegistry().Len() > 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if engine.ResponseRegistry().Len() == 0 {
		t.Fatal("slot was never registered before redeploy")
	}

	// Redeploy with a completely different flow. The old in-flight
	// slot must be drained with the configured shutdown fallback (503).
	v2 := []flow.Flow{{
		ID: "f2", Type: "tab",
		Nodes: []flow.Node{},
	}}
	if err := engine.Deploy(v2, nil, flow.DeployFull); err != nil {
		t.Fatal(err)
	}

	select {
	case r := <-resCh:
		if r.err != nil {
			t.Fatalf("client error: %v", r.err)
		}
		if r.status != http.StatusServiceUnavailable {
			t.Errorf("redeploy drain status: got %d, want 503", r.status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client did not receive a response within 3s of redeploy")
	}
}

// TestEndToEndUnknownPathIs404 verifies that requests to /endpoint/<X>
// where no http-in node has registered return 404 (chi default).
func TestEndToEndUnknownPathIs404(t *testing.T) {
	cfg := &config.Config{HTTPNodeRoot: "/endpoint"}
	engine := flow.NewEngine(cfg)
	engine.Registry().Register("http-in", network.NewHTTPInNode, network.HTTPInTypeInfo())

	tsrv, _ := buildE2EHarness(t, engine)
	defer tsrv.Close()

	if err := engine.Start(); err != nil {
		t.Fatal(err)
	}
	defer engine.Stop()

	resp, err := http.Get(tsrv.URL + "/endpoint/no-such-path")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("got %d, want 404", resp.StatusCode)
	}
}

// stuckNode never produces output — incoming messages are intentionally
// dropped so the http-in handler blocks until the registry is drained.
type stuckNode struct{}

func (n *stuckNode) Init() error                                            { return nil }
func (n *stuckNode) SetSend(_ flow.SendFunc)                                {}
func (n *stuckNode) SetStatus(_ flow.StatusFunc)                            {}
func (n *stuckNode) SetDebug(_ flow.DebugFunc)                              {}
func (n *stuckNode) Start() error                                           { return nil }
func (n *stuckNode) Stop() error                                            { return nil }
func (n *stuckNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	return nil, nil
}

// keep a reference so the build doesn't drop the unused passthrough
// stub if a future test wants it.
var _ = (&passthroughNode{}).HandleMessage
var _ atomic.Bool
