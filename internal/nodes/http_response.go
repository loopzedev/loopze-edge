// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// HTTPResponseNode resolves the response handle on msg.res, writes the
// configured / per-message status, headers, cookies, and body back to
// the original requester, and signals completion to the http-in node
// that's blocked on the slot.
//
// Sink node: 1 input, 0 outputs.
type HTTPResponseNode struct {
	cfg flow.NodeConfig

	send    flow.SendFunc
	status  flow.StatusFunc
	debug   flow.DebugFunc
	errorFn flow.ErrorFunc

	// Injected by the engine via HTTPMuxProvider.
	registry *flow.ResponseRegistry

	// Parsed configuration.
	statusCode int
	headers    map[string]string
}

// NewHTTPResponseNode is the NodeFactory for the "http-response" node type.
func NewHTTPResponseNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &HTTPResponseNode{cfg: cfg}, nil
}

func (n *HTTPResponseNode) Init() error {
	props := n.cfg.Properties

	code := intVal(props, "statusCode", http.StatusOK)
	if code < 100 || code > 599 {
		return fmt.Errorf("http-response %s: statusCode %d out of range", n.cfg.ID, code)
	}
	n.statusCode = code

	if raw, ok := props["headers"].(map[string]any); ok {
		n.headers = make(map[string]string, len(raw))
		for k, v := range raw {
			if s, ok := v.(string); ok {
				n.headers[k] = s
			}
		}
	}
	return nil
}

func (n *HTTPResponseNode) SetSend(fn flow.SendFunc)     { n.send = fn }
func (n *HTTPResponseNode) SetStatus(fn flow.StatusFunc) { n.status = fn }
func (n *HTTPResponseNode) SetDebug(fn flow.DebugFunc)   { n.debug = fn }
func (n *HTTPResponseNode) SetError(fn flow.ErrorFunc)   { n.errorFn = fn }

// SetHTTPMux implements flow.HTTPMuxProvider. http-response only needs
// the registry to resolve handles; the root prefix is irrelevant here.
func (n *HTTPResponseNode) SetHTTPMux(registry *flow.ResponseRegistry, _ string) {
	n.registry = registry
}

func (n *HTTPResponseNode) Start() error {
	if n.registry == nil {
		return fmt.Errorf("http-response %s: response registry not injected", n.cfg.ID)
	}
	slog.Info("http-response started", "node_id", n.cfg.ID, "default_status", n.statusCode)
	return nil
}

func (n *HTTPResponseNode) Stop() error {
	slog.Info("http-response stopped", "node_id", n.cfg.ID)
	return nil
}

// HandleMessage looks up the response handle on msg.res and writes the
// response under sync.Once. Errors from missing / already-completed
// handles are reported through the standard error pipeline; the
// message itself is dropped (sink node).
func (n *HTTPResponseNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, errors.New("http-response: nil message")
	}
	handle, ok := msg.Get("res").(*flow.ResponseHandle)
	if !ok || handle == nil {
		return nil, errors.New("http-response: msg.res missing or not a *flow.ResponseHandle")
	}

	statusCode := n.statusCode
	if v, ok := msg.Get("statusCode").(float64); ok {
		statusCode = int(v)
	} else if v, ok := msg.Get("statusCode").(int); ok {
		statusCode = v
	}

	mergedHeaders := mergeStringMaps(n.headers, extractHeaders(msg))
	cookies := extractCookies(msg)
	payload := msg.Payload()

	_, err := n.registry.Complete(handle.ID(), func(w http.ResponseWriter, _ *http.Request) {
		writeResponse(w, statusCode, mergedHeaders, cookies, payload)
	})
	if err != nil {
		// "already completed" can happen on fan-out where multiple
		// branches end in http-response. We surface it as a catchable
		// warning; the message is dropped (sink). "unknown handle"
		// usually means the slot expired or was drained — same shape.
		return nil, fmt.Errorf("http-response %s: %w", n.cfg.ID, err)
	}
	return nil, nil
}

// writeResponse encodes the body by type, sets default Content-Type
// when the user did not provide one, then writes status + headers +
// cookies + body in the right order.
func writeResponse(w http.ResponseWriter, code int, headers map[string]string, cookies []*http.Cookie, payload any) {
	body, defaultCT := encodeResponseBody(payload)

	for k, v := range headers {
		w.Header().Set(k, v)
	}
	if defaultCT != "" && w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", defaultCT)
	}
	for _, c := range cookies {
		http.SetCookie(w, c)
	}
	w.WriteHeader(code)
	if len(body) > 0 {
		_, _ = w.Write(body)
	}
}

// encodeResponseBody turns msg.payload into bytes, returning a default
// Content-Type that callers should set only if the user didn't specify
// one explicitly via msg.headers / config headers.
func encodeResponseBody(payload any) ([]byte, string) {
	switch v := payload.(type) {
	case nil:
		return nil, ""
	case string:
		return []byte(v), "text/plain; charset=utf-8"
	case []byte:
		return v, "application/octet-stream"
	case []int:
		// Convention from MQTT / http-in: number array represents raw
		// bytes. Reconstruct on write so the wire format is binary,
		// not a JSON array.
		out := make([]byte, len(v))
		for i, n := range v {
			out[i] = byte(n & 0xff)
		}
		return out, "application/octet-stream"
	case []any:
		// Could be a number array decoded from JSON ("data": [1,2,3]).
		// Coerce best-effort; fall back to JSON if it's not numeric.
		if asBytes, ok := numberArrayBytes(v); ok {
			return asBytes, "application/octet-stream"
		}
		b, err := json.Marshal(v)
		if err != nil {
			return []byte(fmt.Sprintf("%v", v)), "text/plain; charset=utf-8"
		}
		return b, "application/json"
	case map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			return []byte(fmt.Sprintf("%v", v)), "text/plain; charset=utf-8"
		}
		return b, "application/json"
	default:
		// Numbers, booleans, structs: stringify via fmt.
		return []byte(fmt.Sprintf("%v", v)), "text/plain; charset=utf-8"
	}
}

// numberArrayBytes attempts to coerce a []any into []byte when every
// element is a numeric byte. Returns ok=false otherwise.
func numberArrayBytes(v []any) ([]byte, bool) {
	out := make([]byte, len(v))
	for i, e := range v {
		switch n := e.(type) {
		case float64:
			if n < 0 || n > 255 {
				return nil, false
			}
			out[i] = byte(n)
		case int:
			if n < 0 || n > 255 {
				return nil, false
			}
			out[i] = byte(n)
		default:
			return nil, false
		}
	}
	return out, true
}

// extractHeaders pulls msg.headers into a normalised map[string]string.
// Accepts both map[string]string and map[string]any shapes.
func extractHeaders(msg *flow.Message) map[string]string {
	raw := msg.Get("headers")
	switch m := raw.(type) {
	case map[string]string:
		return m
	case map[string]any:
		out := make(map[string]string, len(m))
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
		return out
	}
	return nil
}

// extractCookies parses msg.cookies into a slice of *http.Cookie.
// Accepts:
//   - map[string]string : { "name": "value" } shorthand
//   - map[string]any with attribute objects: { "name": {"value":"v","maxAge":n,…} }
func extractCookies(msg *flow.Message) []*http.Cookie {
	raw := msg.Get("cookies")
	if raw == nil {
		return nil
	}
	asAny, ok := raw.(map[string]any)
	if !ok {
		// Try the simple shorthand.
		if m, ok := raw.(map[string]string); ok {
			out := make([]*http.Cookie, 0, len(m))
			for k, v := range m {
				out = append(out, &http.Cookie{Name: k, Value: v})
			}
			return out
		}
		return nil
	}
	out := make([]*http.Cookie, 0, len(asAny))
	for name, val := range asAny {
		switch v := val.(type) {
		case string:
			out = append(out, &http.Cookie{Name: name, Value: v})
		case map[string]any:
			c := &http.Cookie{Name: name}
			if s, ok := v["value"].(string); ok {
				c.Value = s
			}
			if n, ok := v["maxAge"].(float64); ok {
				c.MaxAge = int(n)
			} else if n, ok := v["maxAge"].(int); ok {
				c.MaxAge = n
			}
			if s, ok := v["path"].(string); ok {
				c.Path = s
			}
			if s, ok := v["domain"].(string); ok {
				c.Domain = s
			}
			if b, ok := v["httpOnly"].(bool); ok {
				c.HttpOnly = b
			}
			if b, ok := v["secure"].(bool); ok {
				c.Secure = b
			}
			if s, ok := v["sameSite"].(string); ok {
				switch strings.ToLower(s) {
				case "strict":
					c.SameSite = http.SameSiteStrictMode
				case "lax":
					c.SameSite = http.SameSiteLaxMode
				case "none":
					c.SameSite = http.SameSiteNoneMode
				}
			}
			out = append(out, c)
		}
	}
	return out
}

// mergeStringMaps returns a new map with the union of a and b. Keys in
// b override keys in a (msg headers override config headers per spec).
func mergeStringMaps(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// HTTPResponseTypeInfo returns the NodeTypeInfo for the http-response node.
func HTTPResponseTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "http-response",
		Category:    "network",
		Label:       "HTTP Response",
		Description: "Sends the HTTP response back to the original requester",
		Icon:        "mdi-cloud-upload",
		Defaults: map[string]any{
			"statusCode": 200,
			"headers":    map[string]any{},
		},
		Inputs:  1,
		Outputs: 0,
	}
}
