// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// httpRespHarness wires an HTTPResponseNode for tests.
type httpRespHarness struct {
	t        *testing.T
	node     *HTTPResponseNode
	registry *flow.ResponseRegistry
	rec      *httptest.ResponseRecorder
}

func newHTTPRespHarness(t *testing.T, props map[string]any) *httpRespHarness {
	t.Helper()
	if props == nil {
		props = map[string]any{}
	}
	node, err := NewHTTPResponseNode(flow.NodeConfig{
		ID: t.Name(), Type: "http-response", FlowID: "f1", Properties: props,
	})
	if err != nil {
		t.Fatalf("NewHTTPResponseNode: %v", err)
	}
	hr := node.(*HTTPResponseNode)
	if err := hr.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	hr.SetSend(func(_ int, _ *flow.Message) {})
	hr.SetStatus(noopStatus)
	hr.SetDebug(noopDebug)
	hr.SetError(func(_ error, _ *flow.Message) {})
	rg := flow.NewResponseRegistry()
	hr.SetHTTPMux(rg, "/endpoint")
	if err := hr.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return &httpRespHarness{t: t, node: hr, registry: rg, rec: httptest.NewRecorder()}
}

// registerSlot installs a slot in the registry and returns a message
// with msg.res pointing at the new handle.
func (h *httpRespHarness) registerSlot() (*flow.Message, *flow.ResponseHandle) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	handle, _ := h.registry.Register(h.rec, req, 0, "n1", "f1")
	msg := flow.NewMessage()
	msg.Set("res", handle)
	return msg, handle
}

// ─── Basic happy path ──────────────────────────────────────────────────────

func TestHTTPResponseDefaultStatusAndBody(t *testing.T) {
	h := newHTTPRespHarness(t, nil)
	msg, _ := h.registerSlot()
	msg.SetPayload("hello")

	if _, err := h.node.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if h.rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", h.rec.Code)
	}
	if h.rec.Body.String() != "hello" {
		t.Errorf("body: got %q, want %q", h.rec.Body.String(), "hello")
	}
	if h.rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("default content-type for string: got %q", h.rec.Header().Get("Content-Type"))
	}
}

func TestHTTPResponseStatusCodeFromMsgOverridesConfig(t *testing.T) {
	h := newHTTPRespHarness(t, map[string]any{"statusCode": 200})
	msg, _ := h.registerSlot()
	msg.Set("statusCode", float64(404))
	msg.SetPayload("nope")

	if _, err := h.node.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if h.rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", h.rec.Code)
	}
}

func TestHTTPResponseHeadersMergeMsgWins(t *testing.T) {
	h := newHTTPRespHarness(t, map[string]any{
		"headers": map[string]any{
			"X-Foo":      "config-foo",
			"X-Constant": "always",
		},
	})
	msg, _ := h.registerSlot()
	msg.Set("headers", map[string]any{
		"X-Foo":   "msg-foo",
		"X-Extra": "from-msg",
	})

	_, _ = h.node.HandleMessage(msg)

	if got := h.rec.Header().Get("X-Foo"); got != "msg-foo" {
		t.Errorf("X-Foo: got %q, want msg-foo", got)
	}
	if got := h.rec.Header().Get("X-Constant"); got != "always" {
		t.Errorf("X-Constant: got %q", got)
	}
	if got := h.rec.Header().Get("X-Extra"); got != "from-msg" {
		t.Errorf("X-Extra: got %q", got)
	}
}

// ─── Body encoding by payload type ─────────────────────────────────────────

func TestHTTPResponseMapPayloadEncodesJSON(t *testing.T) {
	h := newHTTPRespHarness(t, nil)
	msg, _ := h.registerSlot()
	msg.SetPayload(map[string]any{"k": "v", "n": 3})

	_, _ = h.node.HandleMessage(msg)

	if h.rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("ct: got %q", h.rec.Header().Get("Content-Type"))
	}
	body := h.rec.Body.String()
	if !strings.Contains(body, `"k":"v"`) || !strings.Contains(body, `"n":3`) {
		t.Errorf("json body: %q", body)
	}
}

func TestHTTPResponseBytePayload(t *testing.T) {
	h := newHTTPRespHarness(t, nil)
	msg, _ := h.registerSlot()
	msg.SetPayload([]byte{0xDE, 0xAD, 0xBE, 0xEF})

	_, _ = h.node.HandleMessage(msg)

	if h.rec.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("ct: got %q", h.rec.Header().Get("Content-Type"))
	}
	if got := h.rec.Body.Bytes(); len(got) != 4 || got[0] != 0xDE {
		t.Errorf("bytes body: got %v", got)
	}
}

func TestHTTPResponseNumberArrayPayload(t *testing.T) {
	h := newHTTPRespHarness(t, nil)
	msg, _ := h.registerSlot()
	msg.SetPayload([]int{0xCA, 0xFE, 0xBA, 0xBE})

	_, _ = h.node.HandleMessage(msg)
	if got := h.rec.Body.Bytes(); len(got) != 4 || got[0] != 0xCA {
		t.Errorf("number-array as bytes: got %v", got)
	}
}

func TestHTTPResponseNilPayloadEmptyBody(t *testing.T) {
	h := newHTTPRespHarness(t, nil)
	msg, _ := h.registerSlot()
	// no SetPayload call → payload is nil

	_, _ = h.node.HandleMessage(msg)
	if h.rec.Body.Len() != 0 {
		t.Errorf("nil payload: body should be empty, got %q", h.rec.Body.String())
	}
}

// ─── Cookies ───────────────────────────────────────────────────────────────

func TestHTTPResponseCookiesShorthand(t *testing.T) {
	h := newHTTPRespHarness(t, nil)
	msg, _ := h.registerSlot()
	msg.Set("cookies", map[string]any{"session": "abc"})

	_, _ = h.node.HandleMessage(msg)
	got := h.rec.Header().Values("Set-Cookie")
	if len(got) == 0 || !strings.HasPrefix(got[0], "session=abc") {
		t.Errorf("cookie: got %v", got)
	}
}

func TestHTTPResponseCookiesAttributes(t *testing.T) {
	h := newHTTPRespHarness(t, nil)
	msg, _ := h.registerSlot()
	msg.Set("cookies", map[string]any{
		"session": map[string]any{
			"value":    "xyz",
			"maxAge":   3600,
			"path":     "/",
			"httpOnly": true,
			"secure":   true,
			"sameSite": "lax",
		},
	})

	_, _ = h.node.HandleMessage(msg)
	got := h.rec.Header().Values("Set-Cookie")
	if len(got) == 0 {
		t.Fatal("no Set-Cookie header")
	}
	c := got[0]
	for _, want := range []string{"session=xyz", "Max-Age=3600", "Path=/", "HttpOnly", "Secure", "SameSite=Lax"} {
		if !strings.Contains(c, want) {
			t.Errorf("Set-Cookie %q missing %q", c, want)
		}
	}
}

// ─── Error cases ───────────────────────────────────────────────────────────

func TestHTTPResponseMissingHandle(t *testing.T) {
	h := newHTTPRespHarness(t, nil)
	msg := flow.NewMessage() // no res

	_, err := h.node.HandleMessage(msg)
	if err == nil {
		t.Fatal("expected error for missing handle")
	}
	if !strings.Contains(err.Error(), "msg.res") {
		t.Errorf("error message: %v", err)
	}
}

func TestHTTPResponseAlreadyCompletedReportsError(t *testing.T) {
	h := newHTTPRespHarness(t, nil)
	msg, handle := h.registerSlot()
	msg.SetPayload("first")

	if _, err := h.node.HandleMessage(msg); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Second call: registry deletes the slot after success, so we get
	// ErrUnknownHandle (same shape from caller's perspective).
	msg2 := flow.NewMessage()
	msg2.Set("res", handle)
	msg2.SetPayload("second")
	_, err := h.node.HandleMessage(msg2)
	if err == nil {
		t.Fatal("second call should error (handle gone)")
	}
	if !errors.Is(err, flow.ErrUnknownHandle) && !errors.Is(err, flow.ErrAlreadyCompleted) {
		t.Errorf("unexpected error: %v", err)
	}

	// First write must remain — the body is "first", not "second".
	if h.rec.Body.String() != "first" {
		t.Errorf("first write must remain: body=%q", h.rec.Body.String())
	}
}

func TestHTTPResponseInitRejectsBadStatus(t *testing.T) {
	n, _ := NewHTTPResponseNode(flow.NodeConfig{ID: "x", Properties: map[string]any{
		"statusCode": 99,
	}})
	if err := n.(*HTTPResponseNode).Init(); err == nil {
		t.Errorf("Init should reject statusCode=99")
	}
}
