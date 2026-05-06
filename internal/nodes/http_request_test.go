// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// httpReqHarness wires an HTTPRequestNode for tests.
type httpReqHarness struct {
	t      *testing.T
	node   *HTTPRequestNode
	status []struct{ fill, text string }
}

func newHTTPReqHarness(t *testing.T, props map[string]any) *httpReqHarness {
	t.Helper()
	if props == nil {
		props = map[string]any{}
	}
	node, err := NewHTTPRequestNode(flow.NodeConfig{
		ID: t.Name(), Type: "http-request", FlowID: "f1", Properties: props,
	})
	if err != nil {
		t.Fatalf("NewHTTPRequestNode: %v", err)
	}
	hr := node.(*HTTPRequestNode)
	if err := hr.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	h := &httpReqHarness{t: t, node: hr}
	hr.SetSend(func(_ int, _ *flow.Message) {})
	hr.SetStatus(func(fill, text string) {
		h.status = append(h.status, struct{ fill, text string }{fill, text})
	})
	hr.SetDebug(noopDebug)
	hr.SetError(func(_ error, _ *flow.Message) {})
	if err := hr.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return h
}

// callOnce sends `msg` through HandleMessage and returns the first
// outgoing message (or nil) and the error.
func (h *httpReqHarness) callOnce(msg *flow.Message) (*flow.Message, error) {
	out, err := h.node.HandleMessage(msg)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 || len(out[0]) == 0 {
		return nil, nil
	}
	return out[0][0], nil
}

// ─── Happy path ────────────────────────────────────────────────────────────

func TestHTTPRequestBasicGetWithJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hello":"world"}`))
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{
		"method": "GET", "url": srv.URL,
	})
	out, err := h.callOnce(flow.NewMessage())
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if v, _ := out.Get("statusCode").(int); v != 200 {
		t.Errorf("statusCode: %v", out.Get("statusCode"))
	}
	payload, _ := out.Payload().(map[string]any)
	if payload["hello"] != "world" {
		t.Errorf("payload: %v", out.Payload())
	}
}

func TestHTTPRequestMustacheURL(t *testing.T) {
	gotPath := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(204)
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{
		"method": "GET", "url": srv.URL + "/u/{{userId}}",
	})
	msg := flow.NewMessage()
	msg.Set("userId", "42")
	if _, err := h.callOnce(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if gotPath != "/u/42" {
		t.Errorf("path: got %q, want /u/42", gotPath)
	}
}

func TestHTTPRequestMsgURLOverridesConfig(t *testing.T) {
	gotPath := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{
		"method": "GET", "url": "",
	})
	msg := flow.NewMessage()
	msg.Set("url", srv.URL+"/from-msg")
	if _, err := h.callOnce(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if gotPath != "/from-msg" {
		t.Errorf("path: %q", gotPath)
	}
}

func TestHTTPRequestMsgMethodOverrides(t *testing.T) {
	gotMethod := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{"method": "GET", "url": srv.URL})
	msg := flow.NewMessage()
	msg.Set("method", "DELETE")
	if _, err := h.callOnce(msg); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "DELETE" {
		t.Errorf("method: got %q", gotMethod)
	}
}

// ─── Body encoding ─────────────────────────────────────────────────────────

func TestHTTPRequestAutoJSONBodyForMapPayload(t *testing.T) {
	var gotCT string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{"method": "POST", "url": srv.URL})
	msg := flow.NewMessage()
	msg.SetPayload(map[string]any{"k": "v"})
	if _, err := h.callOnce(msg); err != nil {
		t.Fatal(err)
	}
	if gotCT != "application/json" {
		t.Errorf("CT: %q", gotCT)
	}
	if !strings.Contains(gotBody, `"k":"v"`) {
		t.Errorf("body: %q", gotBody)
	}
}

func TestHTTPRequestFormBodyEncoding(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{
		"method": "POST", "url": srv.URL, "bodyEncoding": "form",
	})
	msg := flow.NewMessage()
	msg.SetPayload(map[string]any{"a": "1", "b": "two"})
	if _, err := h.callOnce(msg); err != nil {
		t.Fatal(err)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("CT: %q", gotCT)
	}
	if !strings.Contains(gotBody, "a=1") || !strings.Contains(gotBody, "b=two") {
		t.Errorf("form body: %q", gotBody)
	}
}

// ─── Auth ──────────────────────────────────────────────────────────────────

func TestHTTPRequestBasicAuth(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{
		"method": "GET", "url": srv.URL,
		"auth": map[string]any{"type": "basic", "username": "u", "password": "p"},
	})
	if _, err := h.callOnce(flow.NewMessage()); err != nil {
		t.Fatal(err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("u:p"))
	if got != want {
		t.Errorf("Authorization: %q, want %q", got, want)
	}
}

func TestHTTPRequestBearerAuth(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{
		"method": "GET", "url": srv.URL,
		"auth": map[string]any{"type": "bearer", "token": "tok123"},
	})
	if _, err := h.callOnce(flow.NewMessage()); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer tok123" {
		t.Errorf("Authorization: %q", got)
	}
}

// ─── Error mode and timeout ────────────────────────────────────────────────

func TestHTTPRequestNon2xxPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{"method": "GET", "url": srv.URL})
	out, err := h.callOnce(flow.NewMessage())
	if err != nil {
		t.Fatalf("passthrough: should not error on 404, got %v", err)
	}
	if v, _ := out.Get("statusCode").(int); v != 404 {
		t.Errorf("statusCode: %v", out.Get("statusCode"))
	}
}

func TestHTTPRequestNon2xxAsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{
		"method": "GET", "url": srv.URL, "errorMode": "error",
	})
	_, err := h.callOnce(flow.NewMessage())
	if err == nil {
		t.Fatal("errorMode=error should fail on 500")
	}
}

func TestHTTPRequestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{
		"method": "GET", "url": srv.URL, "timeout": 1, // 1 second min — set client timeout below
	})
	// Override the client to a short timeout for fast test
	h.node.client.Timeout = 50 * time.Millisecond
	_, err := h.callOnce(flow.NewMessage())
	if err == nil {
		t.Fatal("timeout: expected error")
	}
}

// ─── Response decoding ─────────────────────────────────────────────────────

func TestHTTPRequestJSONResponseAuto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`[1,2,3]`))
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{"method": "GET", "url": srv.URL})
	out, err := h.callOnce(flow.NewMessage())
	if err != nil {
		t.Fatal(err)
	}
	arr, ok := out.Payload().([]any)
	if !ok || len(arr) != 3 {
		t.Errorf("expected []any of length 3, got %T %v", out.Payload(), out.Payload())
	}
}

func TestHTTPRequestStringResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("plain hello"))
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{
		"method": "GET", "url": srv.URL, "responseFormat": "string",
	})
	out, err := h.callOnce(flow.NewMessage())
	if err != nil {
		t.Fatal(err)
	}
	if out.Payload() != "plain hello" {
		t.Errorf("payload: %v", out.Payload())
	}
}

// ─── Headers and query ─────────────────────────────────────────────────────

func TestHTTPRequestHeadersFromConfigAndMsg(t *testing.T) {
	var hConfig, hMsg string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hConfig = r.Header.Get("X-From-Config")
		hMsg = r.Header.Get("X-From-Msg")
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{
		"method": "GET", "url": srv.URL,
		"headers": map[string]any{"X-From-Config": "cfg"},
	})
	msg := flow.NewMessage()
	msg.Set("headers", map[string]any{"X-From-Msg": "m"})
	if _, err := h.callOnce(msg); err != nil {
		t.Fatal(err)
	}
	if hConfig != "cfg" {
		t.Errorf("config header: %q", hConfig)
	}
	if hMsg != "m" {
		t.Errorf("msg header: %q", hMsg)
	}
}

func TestHTTPRequestQueryMergeMsgWins(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
	}))
	defer srv.Close()

	h := newHTTPReqHarness(t, map[string]any{
		"method": "GET", "url": srv.URL + "?a=base",
		"query": map[string]any{"b": "from-config"},
	})
	msg := flow.NewMessage()
	msg.Set("query", map[string]any{"b": "from-msg"})
	if _, err := h.callOnce(msg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "b=from-msg") || !strings.Contains(got, "a=base") {
		t.Errorf("query: %q", got)
	}
}

// ─── TLS insecure ──────────────────────────────────────────────────────────

func TestHTTPRequestTLSInsecureAcceptsSelfSigned(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// First, without insecure flag → must fail.
	hStrict := newHTTPReqHarness(t, map[string]any{"method": "GET", "url": srv.URL})
	_, err := hStrict.callOnce(flow.NewMessage())
	if err == nil {
		t.Fatal("expected TLS error against self-signed server")
	}

	// Now with insecure → succeeds.
	hLax := newHTTPReqHarness(t, map[string]any{
		"method": "GET", "url": srv.URL, "tlsInsecure": true,
	})
	if _, err := hLax.callOnce(flow.NewMessage()); err != nil {
		t.Fatalf("insecure should accept self-signed: %v", err)
	}
}

// ─── Init validation ───────────────────────────────────────────────────────

func TestHTTPRequestInitValidatesEnums(t *testing.T) {
	cases := []struct {
		name  string
		props map[string]any
		bad   bool
	}{
		{"valid", map[string]any{"method": "GET", "url": "https://x"}, false},
		{"bad method", map[string]any{"method": "BOGUS", "url": "x"}, true},
		{"bad responseFormat", map[string]any{"method": "GET", "responseFormat": "xml"}, true},
		{"bad bodyEncoding", map[string]any{"method": "GET", "bodyEncoding": "yaml"}, true},
		{"bad errorMode", map[string]any{"method": "GET", "errorMode": "panic"}, true},
		{"bad URL template", map[string]any{"method": "GET", "url": "{{unclosed"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, _ := NewHTTPRequestNode(flow.NodeConfig{ID: c.name, Properties: c.props})
			err := n.(*HTTPRequestNode).Init()
			if c.bad && err == nil {
				t.Errorf("expected error for %s", c.name)
			}
			if !c.bad && err != nil {
				t.Errorf("unexpected error for %s: %v", c.name, err)
			}
		})
	}
}

// ─── Type info sanity ──────────────────────────────────────────────────────

func TestHTTPRequestTypeInfoStable(t *testing.T) {
	info := HTTPRequestTypeInfo()
	if info.Type != "http-request" {
		t.Errorf("type: %s", info.Type)
	}
	if info.Inputs != 1 || info.Outputs != 1 {
		t.Errorf("ports: %d/%d", info.Inputs, info.Outputs)
	}
	if _, err := json.Marshal(info); err != nil {
		t.Errorf("type info not JSON-marshallable: %v", err)
	}
}

// silence unused import warning when tls.Config isn't reached in a build.
var _ = tls.Config{}
