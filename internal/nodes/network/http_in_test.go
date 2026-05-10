// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package network

import (
	"github.com/loopzedev/loopze-edge/internal/nodes/nodestest"
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// httpInHarness wires an HTTPInNode for tests: collector for sent
// messages, status capture, error capture, fresh response registry.
type httpInHarness struct {
	t        *testing.T
	node     *HTTPInNode
	col      *nodestest.Collector
	registry *flow.ResponseRegistry

	statusMu sync.Mutex
	status   []struct{ fill, text string }
	errMu    sync.Mutex
	errs     []error
}

func newHTTPInHarness(t *testing.T, props map[string]any) *httpInHarness {
	t.Helper()
	node, err := NewHTTPInNode(flow.NodeConfig{
		ID:         t.Name(),
		Type:       "http-in",
		FlowID:     "f1",
		Properties: props,
	})
	if err != nil {
		t.Fatalf("NewHTTPInNode: %v", err)
	}
	hin := node.(*HTTPInNode)
	if err := hin.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	h := &httpInHarness{t: t, node: hin, col: &nodestest.Collector{}, registry: flow.NewResponseRegistry()}
	hin.SetSend(h.col.Send)
	hin.SetStatus(func(fill, text string) {
		h.statusMu.Lock()
		h.status = append(h.status, struct{ fill, text string }{fill, text})
		h.statusMu.Unlock()
	})
	hin.SetDebug(noopDebug)
	hin.SetError(func(err error, _ *flow.Message) {
		h.errMu.Lock()
		h.errs = append(h.errs, err)
		h.errMu.Unlock()
	})
	hin.SetHTTPMux(h.registry, "/endpoint")
	if err := hin.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return h
}

// stop tears the harness down.
func (h *httpInHarness) stop() {
	_ = h.node.Stop()
	h.registry.Stop()
}

// invokeAndAutoRespond fires the node's primary handler against (req)
// in a goroutine and, in parallel, drains the registry as soon as the
// flow message arrives by completing the slot with a 200 OK. Returns
// the recorder once the handler unblocks.
func (h *httpInHarness) invokeAndAutoRespond(req *http.Request, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Wait for the handler to fire the message.
		for {
			if h.col.Count() > 0 {
				break
			}
			select {
			case <-time.After(2 * time.Second):
				h.t.Errorf("handler never produced a message")
				return
			default:
				time.Sleep(2 * time.Millisecond)
			}
		}
		msg := h.col.Last()
		handle, _ := msg.Get("res").(*flow.ResponseHandle)
		if handle == nil {
			h.t.Error("msg.res missing or not *ResponseHandle")
			return
		}
		_, err := h.registry.Complete(handle.ID(), func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			if body != "" {
				_, _ = w.Write([]byte(body))
			}
		})
		if err != nil {
			h.t.Errorf("complete: %v", err)
		}
	}()

	h.node.handle(rec, req)
	<-done
	return rec
}

// withChiRoute attaches a chi RouteContext that has the given URL
// params, so the handler's chi.URLParam lookups work in unit tests.
func withChiRoute(req *http.Request, kv map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range kv {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// ─── Init / route registration ─────────────────────────────────────────────

func TestHTTPInInitValidatesMethod(t *testing.T) {
	_, err := NewHTTPInNode(flow.NodeConfig{ID: "x", Properties: map[string]any{
		"method": "FOO", "path": "/x",
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Init is on the concrete node — instantiate then call.
	n, _ := NewHTTPInNode(flow.NodeConfig{ID: "x", Properties: map[string]any{
		"method": "FOO", "path": "/x",
	}})
	if err := n.(*HTTPInNode).Init(); err == nil {
		t.Errorf("Init should reject method=FOO")
	}
}

func TestHTTPInInitValidatesPath(t *testing.T) {
	cases := []struct {
		path string
		bad  bool
	}{
		{"/x", false},
		{"/devices/:id", false},
		{"", true},
		{"x", true},        // missing leading slash
		{"  /x ", false},   // whitespace trimmed
	}
	for _, c := range cases {
		n, _ := NewHTTPInNode(flow.NodeConfig{ID: "x", Properties: map[string]any{
			"method": "GET", "path": c.path,
		}})
		err := n.(*HTTPInNode).Init()
		if c.bad && err == nil {
			t.Errorf("path %q: expected init error", c.path)
		}
		if !c.bad && err != nil {
			t.Errorf("path %q: unexpected init error %v", c.path, err)
		}
	}
}

func TestHTTPInRoutesContainsConfigured(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{"method": "POST", "path": "/webhook"})
	defer h.stop()
	specs := h.node.HTTPRoutes()
	if len(specs) != 1 {
		t.Fatalf("specs: got %d, want 1 (no CORS configured)", len(specs))
	}
	if specs[0].Method != "POST" || specs[0].Path != "/webhook" {
		t.Errorf("spec mismatch: got %+v", specs[0])
	}
}

func TestHTTPInRoutesAddsOptionsForCORS(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{
		"method": "POST", "path": "/webhook",
		"cors": map[string]any{"origins": []any{"*"}},
	})
	defer h.stop()
	specs := h.node.HTTPRoutes()
	if len(specs) != 2 {
		t.Fatalf("specs: got %d, want 2 (POST + OPTIONS for CORS)", len(specs))
	}
	hasOptions := false
	for _, s := range specs {
		if s.Method == http.MethodOptions {
			hasOptions = true
		}
	}
	if !hasOptions {
		t.Errorf("specs missing OPTIONS preflight route: %+v", specs)
	}
}

func TestHTTPInStartListeningStatus(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{"method": "POST", "path": "/webhook"})
	defer h.stop()
	h.statusMu.Lock()
	defer h.statusMu.Unlock()
	if len(h.status) == 0 {
		t.Fatal("Start did not emit a status update")
	}
	last := h.status[len(h.status)-1]
	if last.fill != "green" || !strings.Contains(last.text, "POST /endpoint/webhook") {
		t.Errorf("listening status: got %+v", last)
	}
}

func TestHTTPInOnHTTPRouteConflictTurnsRed(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{"method": "POST", "path": "/webhook"})
	defer h.stop()

	h.node.OnHTTPRouteConflict("duplicate POST /webhook")
	h.statusMu.Lock()
	last := h.status[len(h.status)-1]
	h.statusMu.Unlock()
	if last.fill != "red" || !strings.Contains(last.text, "duplicate") {
		t.Errorf("conflict status: got %+v", last)
	}
}

// ─── Body parsing ──────────────────────────────────────────────────────────

func TestHTTPInBodyParseAutoJSON(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{"method": "POST", "path": "/x", "bodyParse": "auto"})
	defer h.stop()

	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"hello":"world","n":3}`))
	req.Header.Set("Content-Type", "application/json")
	rec := h.invokeAndAutoRespond(req, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("recorder status: %d", rec.Code)
	}
	msg := h.col.Last()
	if msg == nil {
		t.Fatal("no message")
	}
	payload, _ := msg.Payload().(map[string]any)
	if payload == nil {
		t.Fatalf("payload not parsed map: got %T = %v", msg.Payload(), msg.Payload())
	}
	if payload["hello"] != "world" {
		t.Errorf("payload.hello: got %v", payload["hello"])
	}
}

func TestHTTPInBodyParseAutoFormURLEncoded(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{"method": "POST", "path": "/x", "bodyParse": "auto"})
	defer h.stop()

	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("a=1&b=two"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.invokeAndAutoRespond(req, "")

	payload, _ := h.col.Last().Payload().(map[string]string)
	if payload == nil {
		t.Fatalf("form payload not map[string]string: %T", h.col.Last().Payload())
	}
	if payload["a"] != "1" || payload["b"] != "two" {
		t.Errorf("form fields: got %+v", payload)
	}
}

func TestHTTPInBodyParseAutoMultipart(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{"method": "POST", "path": "/x", "bodyParse": "auto"})
	defer h.stop()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("subject", "hello")
	fileWriter, _ := mw.CreateFormFile("file", "report.bin")
	_, _ = fileWriter.Write([]byte{0xDE, 0xAD, 0xBE, 0xEF})
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/x", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	h.invokeAndAutoRespond(req, "")

	payload, ok := h.col.Last().Payload().(map[string]any)
	if !ok {
		t.Fatalf("multipart payload not map: %T", h.col.Last().Payload())
	}
	if payload["subject"] != "hello" {
		t.Errorf("subject: got %v", payload["subject"])
	}
	file, ok := payload["file"].(map[string]any)
	if !ok {
		t.Fatalf("file field not map: %T", payload["file"])
	}
	if file["filename"] != "report.bin" {
		t.Errorf("filename: got %v", file["filename"])
	}
	data, _ := file["data"].([]int)
	if len(data) != 4 || data[0] != 0xDE {
		t.Errorf("file data: got %v", data)
	}
}

func TestHTTPInBodyParseJSONBadJSONReturns400(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{"method": "POST", "path": "/x", "bodyParse": "json"})
	defer h.stop()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")

	h.node.handle(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad-json status: got %d, want 400", rec.Code)
	}
	if h.col.Count() != 0 {
		t.Errorf("bad-json should not produce a flow message, got %d", h.col.Count())
	}
	h.errMu.Lock()
	gotErr := len(h.errs) > 0
	h.errMu.Unlock()
	if !gotErr {
		t.Errorf("bad-json should have raised a catchable error")
	}
}

func TestHTTPInBodyParseNoneSkipsBody(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{"method": "POST", "path": "/x", "bodyParse": "none"})
	defer h.stop()

	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"a":1}`))
	req.Header.Set("Content-Type", "application/json")
	h.invokeAndAutoRespond(req, "")

	if p := h.col.Last().Payload(); p != nil {
		t.Errorf("bodyParse=none should leave payload unset, got %v", p)
	}
}

func TestHTTPInBodyParseBufferGivesNumberArray(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{"method": "POST", "path": "/x", "bodyParse": "buffer"})
	defer h.stop()

	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader([]byte{0x01, 0x02, 0x03}))
	h.invokeAndAutoRespond(req, "")

	payload, ok := h.col.Last().Payload().([]int)
	if !ok {
		t.Fatalf("payload not []int: %T", h.col.Last().Payload())
	}
	if len(payload) != 3 || payload[1] != 2 {
		t.Errorf("buffer payload: got %v", payload)
	}
}

func TestHTTPInBodyOversizeReturns413(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{
		"method": "POST", "path": "/x", "bodyParse": "string", "maxBodyBytes": 4,
	})
	defer h.stop()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("toolarge"))
	h.node.handle(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize: got %d, want 413", rec.Code)
	}
}

// ─── Request envelope ──────────────────────────────────────────────────────

func TestHTTPInRequestEnvelopeFields(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{"method": "GET", "path": "/devices/:id", "bodyParse": "none"})
	defer h.stop()

	req := httptest.NewRequest(http.MethodGet, "/devices/42?force=1&force=2&token=x", nil)
	req.Header.Set("X-Custom", "v")
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc"})
	req = withChiRoute(req, map[string]string{"id": "42"})
	h.invokeAndAutoRespond(req, "")

	envelope, ok := h.col.Last().Get("req").(map[string]any)
	if !ok {
		t.Fatalf("req envelope missing: %T", h.col.Last().Get("req"))
	}
	if envelope["method"] != "GET" {
		t.Errorf("method: %v", envelope["method"])
	}
	if envelope["path"] != "/devices/42" {
		t.Errorf("path: %v", envelope["path"])
	}

	params, _ := envelope["params"].(map[string]string)
	if params["id"] != "42" {
		t.Errorf("params: got %+v", params)
	}

	query, ok := envelope["query"].(url.Values)
	if !ok {
		t.Fatalf("query envelope type: got %T", envelope["query"])
	}
	if forceVals := query["force"]; len(forceVals) != 2 {
		t.Errorf("repeated query key not preserved: got %+v", query)
	}
	_ = query

	headers, _ := envelope["headers"].(map[string]string)
	if headers["x-custom"] != "v" {
		t.Errorf("headers (lowercased): got %+v", headers)
	}

	cookies, _ := envelope["cookies"].(map[string]string)
	if cookies["session"] != "abc" {
		t.Errorf("cookies: got %+v", cookies)
	}
}

func TestHTTPInMsgResIsHandle(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{"method": "GET", "path": "/x", "bodyParse": "none"})
	defer h.stop()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	h.invokeAndAutoRespond(req, "ok")

	handle, ok := h.col.Last().Get("res").(*flow.ResponseHandle)
	if !ok || handle == nil || handle.ID() == "" {
		t.Errorf("msg.res: got %v (type %T)", h.col.Last().Get("res"), h.col.Last().Get("res"))
	}
}

// ─── CORS preflight ────────────────────────────────────────────────────────

func TestHTTPInCORSPreflightShortCircuits(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{
		"method": "POST", "path": "/webhook",
		"cors": map[string]any{
			"origins":     []any{"https://example.com"},
			"headers":     []any{"content-type", "authorization"},
			"credentials": true,
			"maxAge":      900,
		},
	})
	defer h.stop()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/webhook", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type,authorization")

	h.node.handlePreflight(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight code: %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Errorf("ACAO: %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("credentials header missing")
	}
	if !strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), "content-type") {
		t.Errorf("headers: %q", rec.Header().Get("Access-Control-Allow-Headers"))
	}
	if rec.Header().Get("Access-Control-Max-Age") != "900" {
		t.Errorf("max-age: %q", rec.Header().Get("Access-Control-Max-Age"))
	}
	if h.col.Count() != 0 {
		t.Errorf("preflight should NOT emit a flow message, got %d", h.col.Count())
	}
}

func TestHTTPInCORSWildcardOrigin(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{
		"method": "GET", "path": "/x",
		"cors": map[string]any{"origins": []any{"*"}},
	})
	defer h.stop()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://anywhere.example")
	h.invokeAndAutoRespond(req, "")

	// The recorder used by invokeAndAutoRespond is created inside the
	// helper; we don't have direct access. Instead we re-fire through
	// applyCORSHeaders directly to verify the wildcard logic.
	rec := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	r2.Header.Set("Origin", "https://x.example")
	h.node.applyCORSHeaders(rec, r2)
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("wildcard ACAO: got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

// ─── Response timeout ──────────────────────────────────────────────────────

func TestHTTPInResponseTimeoutFires504(t *testing.T) {
	h := newHTTPInHarness(t, map[string]any{
		"method":          "GET",
		"path":            "/x",
		"bodyParse":       "none",
		"responseTimeout": 1, // 1 second
	})
	defer h.stop()

	// Faster sweep so the test doesn't drag.
	h.registry.SweepInterval = 5 * time.Millisecond
	h.registry.Stop()
	h.registry.Run()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)

	doneRecorder := make(chan struct{})
	go func() {
		h.node.handle(rec, req)
		close(doneRecorder)
	}()

	// Wait until the slot is registered (handler is now blocked on
	// <-done). Only then jump the clock past the deadline so the
	// computed deadline (real now + 1s) is in the simulated past.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.registry.Len() > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if h.registry.Len() == 0 {
		t.Fatal("slot was never registered")
	}
	advanced := time.Now().Add(2 * time.Second)
	setRegistryNow(h.registry, func() time.Time { return advanced })

	select {
	case <-doneRecorder:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not unblock within 2s")
	}
	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("timeout fallback: got %d, want 504", rec.Code)
	}
}

// ─── helpers ───────────────────────────────────────────────────────────────

// setRegistryNow replaces the unexported now field on a
// flow.ResponseRegistry so tests can simulate the sweeper firing.
// Implemented via a small helper method on the test side (the field is
// unexported but the registry is in the same module).
func setRegistryNow(rg *flow.ResponseRegistry, fn func() time.Time) {
	// This is a deliberate test-only escape hatch implemented by the
	// flow package via reflection-free assignment in
	// SetTimeForTest below.
	rg.SetNowForTest(fn)
}

