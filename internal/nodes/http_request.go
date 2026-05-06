// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cbroglie/mustache"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// Outbound body encoding modes.
const (
	bodyEncodingAuto = "auto"
	bodyEncodingJSON = "json"
	bodyEncodingForm = "form"
	bodyEncodingText = "text"
	bodyEncodingNone = "none"
)

// Response format modes (decoding the response body into msg.payload).
const (
	respFormatAuto   = "auto"
	respFormatString = "string"
	respFormatJSON   = "json"
	respFormatBuffer = "buffer"
)

// errorMode values.
const (
	errModePassthrough = "passthrough"
	errModeError       = "error"
)

const (
	defaultRequestTimeout = 30 * time.Second
	maxRedirects          = 10
)

// HTTPRequestNode performs an outbound HTTP request and emits the
// response on output 0. Synchronous in v1: the node loop blocks on the
// client call so downstream ordering is deterministic.
type HTTPRequestNode struct {
	cfg flow.NodeConfig

	send    flow.SendFunc
	status  flow.StatusFunc
	debug   flow.DebugFunc
	errorFn flow.ErrorFunc

	// Parsed configuration.
	method          string
	useMsgMethod    bool
	urlTemplate     string
	urlParsed       *mustache.Template
	responseFormat  string
	bodyEncoding    string
	headers         map[string]string
	query           map[string]string
	auth            *requestAuthCfg
	timeout         time.Duration
	followRedirects bool
	tlsInsecure     bool
	errorMode       string

	client *http.Client
}

type requestAuthCfg struct {
	kind     string // "none", "basic", "bearer"
	username string
	password string
	token    string
}

// NewHTTPRequestNode is the NodeFactory for the "http-request" node type.
func NewHTTPRequestNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &HTTPRequestNode{cfg: cfg}, nil
}

func (n *HTTPRequestNode) Init() error {
	props := n.cfg.Properties

	method := strings.ToUpper(strings.TrimSpace(stringVal(props, "method", "GET")))
	if method == "USE MSG.METHOD" || method == "MSG" {
		n.useMsgMethod = true
		method = "GET"
	}
	if !isValidMethod(method) {
		return fmt.Errorf("http-request %s: invalid method %q", n.cfg.ID, method)
	}
	n.method = method

	n.urlTemplate = strings.TrimSpace(stringVal(props, "url", ""))
	if n.urlTemplate != "" {
		parsed, err := mustache.ParseString(n.urlTemplate)
		if err != nil {
			return fmt.Errorf("http-request %s: url template parse: %w", n.cfg.ID, err)
		}
		n.urlParsed = parsed
	}

	n.responseFormat = strings.ToLower(stringVal(props, "responseFormat", respFormatAuto))
	switch n.responseFormat {
	case respFormatAuto, respFormatString, respFormatJSON, respFormatBuffer:
	default:
		return fmt.Errorf("http-request %s: invalid responseFormat %q", n.cfg.ID, n.responseFormat)
	}

	n.bodyEncoding = strings.ToLower(stringVal(props, "bodyEncoding", bodyEncodingAuto))
	switch n.bodyEncoding {
	case bodyEncodingAuto, bodyEncodingJSON, bodyEncodingForm, bodyEncodingText, bodyEncodingNone:
	default:
		return fmt.Errorf("http-request %s: invalid bodyEncoding %q", n.cfg.ID, n.bodyEncoding)
	}

	if raw, ok := props["headers"].(map[string]any); ok {
		n.headers = make(map[string]string, len(raw))
		for k, v := range raw {
			if s, ok := v.(string); ok {
				n.headers[k] = s
			}
		}
	}
	if raw, ok := props["query"].(map[string]any); ok {
		n.query = make(map[string]string, len(raw))
		for k, v := range raw {
			if s, ok := v.(string); ok {
				n.query[k] = s
			}
		}
	}

	if raw, ok := props["auth"].(map[string]any); ok {
		n.auth = &requestAuthCfg{
			kind:     stringVal(raw, "type", "none"),
			username: stringVal(raw, "username", ""),
			password: stringVal(raw, "password", ""),
			token:    stringVal(raw, "token", ""),
		}
	}

	timeoutSec := intVal(props, "timeout", int(defaultRequestTimeout/time.Second))
	if timeoutSec <= 0 {
		timeoutSec = int(defaultRequestTimeout / time.Second)
	}
	n.timeout = time.Duration(timeoutSec) * time.Second

	n.followRedirects = true
	if v, ok := props["followRedirects"].(bool); ok {
		n.followRedirects = v
	}
	if v, ok := props["tlsInsecure"].(bool); ok {
		n.tlsInsecure = v
	}

	n.errorMode = strings.ToLower(stringVal(props, "errorMode", errModePassthrough))
	if n.errorMode != errModePassthrough && n.errorMode != errModeError {
		return fmt.Errorf("http-request %s: invalid errorMode %q", n.cfg.ID, n.errorMode)
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	if n.tlsInsecure {
		// nosec G402: deliberately user-controlled per the
		// tlsInsecure config flag; logged at WARN on every deploy.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		slog.Warn("http-request: TLS verification disabled",
			"node_id", n.cfg.ID, "url_template", n.urlTemplate,
		)
	}
	n.client = &http.Client{
		Transport: transport,
		Timeout:   n.timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !n.followRedirects {
				return http.ErrUseLastResponse
			}
			if len(via) >= maxRedirects {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}
	return nil
}

func (n *HTTPRequestNode) SetSend(fn flow.SendFunc)     { n.send = fn }
func (n *HTTPRequestNode) SetStatus(fn flow.StatusFunc) { n.status = fn }
func (n *HTTPRequestNode) SetDebug(fn flow.DebugFunc)   { n.debug = fn }
func (n *HTTPRequestNode) SetError(fn flow.ErrorFunc)   { n.errorFn = fn }

func (n *HTTPRequestNode) Start() error {
	slog.Info("http-request started",
		"node_id", n.cfg.ID, "method", n.method, "url", n.urlTemplate,
		"timeout", n.timeout, "tls_insecure", n.tlsInsecure,
	)
	return nil
}

func (n *HTTPRequestNode) Stop() error {
	if n.client != nil {
		n.client.CloseIdleConnections()
	}
	slog.Info("http-request stopped", "node_id", n.cfg.ID)
	return nil
}

// HandleMessage performs the request and emits the response on output 0.
func (n *HTTPRequestNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		msg = flow.NewMessage()
	}

	method := n.resolveMethod(msg)
	rawURL, err := n.resolveURL(msg)
	if err != nil {
		n.setStatus("red", "url template error")
		return nil, fmt.Errorf("http-request %s: %w", n.cfg.ID, err)
	}
	if rawURL == "" {
		return nil, fmt.Errorf("http-request %s: empty URL", n.cfg.ID)
	}

	finalURL, err := n.applyQuery(rawURL, msg)
	if err != nil {
		return nil, fmt.Errorf("http-request %s: %w", n.cfg.ID, err)
	}

	body, contentType, err := n.encodeBody(msg)
	if err != nil {
		return nil, fmt.Errorf("http-request %s: encode body: %w", n.cfg.ID, err)
	}

	req, err := http.NewRequest(method, finalURL, body)
	if err != nil {
		return nil, fmt.Errorf("http-request %s: build request: %w", n.cfg.ID, err)
	}

	n.applyHeaders(req, contentType, msg)
	n.applyAuth(req)

	start := time.Now()
	resp, err := n.client.Do(req)
	if err != nil {
		n.setStatus("yellow", truncateStatus(err.Error(), 40))
		return nil, fmt.Errorf("http-request %s: %w", n.cfg.ID, err)
	}
	defer resp.Body.Close()

	out, err := n.buildResponseMessage(msg, resp)
	if err != nil {
		n.setStatus("red", "decode error")
		return nil, fmt.Errorf("http-request %s: %w", n.cfg.ID, err)
	}

	elapsed := time.Since(start)
	n.setStatus("green", fmt.Sprintf("%d · %dms", resp.StatusCode, elapsed.Milliseconds()))

	if n.errorMode == errModeError && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		return nil, fmt.Errorf("http-request %s: non-2xx status %d", n.cfg.ID, resp.StatusCode)
	}

	return [][]*flow.Message{{out}}, nil
}

// resolveMethod returns the request method, honouring msg.method when
// the node is configured to use it (or when the static method is set
// and msg.method is empty, msg.method still wins per spec).
func (n *HTTPRequestNode) resolveMethod(msg *flow.Message) string {
	if v, ok := msg.Get("method").(string); ok && v != "" {
		return strings.ToUpper(v)
	}
	if n.useMsgMethod {
		return "GET"
	}
	return n.method
}

// resolveURL renders the configured URL template against msg, falling
// back to msg.url if no template is configured.
func (n *HTTPRequestNode) resolveURL(msg *flow.Message) (string, error) {
	if n.urlParsed != nil {
		return n.urlParsed.Render(msg.DataView())
	}
	if v, ok := msg.Get("url").(string); ok {
		return v, nil
	}
	return "", nil
}

// applyQuery merges msg.query (if any) into the URL's query string.
// msg.query values override any pre-existing keys from the URL or the
// static config.
func (n *HTTPRequestNode) applyQuery(rawURL string, msg *flow.Message) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	q := parsed.Query()

	for k, v := range n.query {
		q.Set(k, v)
	}
	switch m := msg.Get("query").(type) {
	case map[string]string:
		for k, v := range m {
			q.Set(k, v)
		}
	case map[string]any:
		for k, v := range m {
			if s, ok := v.(string); ok {
				q.Set(k, s)
			}
		}
	case url.Values:
		for k, vs := range m {
			q[k] = vs
		}
	}

	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}

// encodeBody produces the request body and a default Content-Type
// based on the configured bodyEncoding mode.
func (n *HTTPRequestNode) encodeBody(msg *flow.Message) (io.Reader, string, error) {
	payload := msg.Payload()

	switch n.bodyEncoding {
	case bodyEncodingNone:
		return nil, "", nil
	case bodyEncodingText:
		if payload == nil {
			return nil, "", nil
		}
		return strings.NewReader(fmt.Sprintf("%v", payload)), "text/plain; charset=utf-8", nil
	case bodyEncodingJSON:
		if payload == nil {
			return nil, "", nil
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, "", fmt.Errorf("marshal: %w", err)
		}
		return bytes.NewReader(b), "application/json", nil
	case bodyEncodingForm:
		m, ok := payload.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("form encoding requires map payload, got %T", payload)
		}
		v := url.Values{}
		for k, val := range m {
			v.Set(k, fmt.Sprintf("%v", val))
		}
		return strings.NewReader(v.Encode()), "application/x-www-form-urlencoded", nil
	}

	// Auto.
	switch p := payload.(type) {
	case nil:
		return nil, "", nil
	case string:
		return strings.NewReader(p), "", nil
	case []byte:
		return bytes.NewReader(p), "application/octet-stream", nil
	case []int:
		out := make([]byte, len(p))
		for i, x := range p {
			out[i] = byte(x & 0xff)
		}
		return bytes.NewReader(out), "application/octet-stream", nil
	case map[string]any, []any:
		b, err := json.Marshal(p)
		if err != nil {
			return nil, "", fmt.Errorf("marshal: %w", err)
		}
		return bytes.NewReader(b), "application/json", nil
	default:
		return strings.NewReader(fmt.Sprintf("%v", p)), "text/plain; charset=utf-8", nil
	}
}

// applyHeaders sets static config headers, msg.headers (which override
// config), and the default Content-Type derived from bodyEncoding when
// the user did not provide one.
func (n *HTTPRequestNode) applyHeaders(req *http.Request, defaultCT string, msg *flow.Message) {
	for k, v := range n.headers {
		req.Header.Set(k, v)
	}
	switch m := msg.Get("headers").(type) {
	case map[string]string:
		for k, v := range m {
			req.Header.Set(k, v)
		}
	case map[string]any:
		for k, v := range m {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}
	if defaultCT != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", defaultCT)
	}
}

// applyAuth attaches the configured authentication header to req.
func (n *HTTPRequestNode) applyAuth(req *http.Request) {
	if n.auth == nil {
		return
	}
	switch n.auth.kind {
	case "basic":
		if n.auth.username != "" || n.auth.password != "" {
			req.SetBasicAuth(n.auth.username, n.auth.password)
		}
	case "bearer":
		if n.auth.token != "" {
			req.Header.Set("Authorization", "Bearer "+n.auth.token)
		}
	}
}

// buildResponseMessage assembles the outgoing message from the response.
// Preserves any preexisting fields on the inbound msg that aren't
// shadowed by the new fields (statusCode, headers, payload, responseUrl).
func (n *HTTPRequestNode) buildResponseMessage(in *flow.Message, resp *http.Response) (*flow.Message, error) {
	out := in.COWClone()
	out.Set("statusCode", resp.StatusCode)
	out.Set("responseUrl", resp.Request.URL.String())

	headers := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[strings.ToLower(k)] = v[0]
		}
	}
	out.Set("headers", headers)

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	payload, parseErr := decodeResponseBody(bodyBytes, resp.Header.Get("Content-Type"), n.responseFormat)
	if parseErr != nil {
		out.Set("parseError", parseErr.Error())
		out.SetPayload(string(bodyBytes))
	} else {
		out.SetPayload(payload)
	}
	return out, nil
}

// decodeResponseBody renders the response body according to
// responseFormat. Returns parseErr (non-nil but body still usable) when
// JSON decoding fails in auto/json mode — callers fall back to string.
func decodeResponseBody(body []byte, contentType, mode string) (any, error) {
	mediaType := mediaTypeOf(contentType)

	switch mode {
	case respFormatString:
		return string(body), nil
	case respFormatBuffer:
		return bytesToNumberArray(body), nil
	case respFormatJSON:
		var v any
		if len(bytes.TrimSpace(body)) == 0 {
			return nil, nil
		}
		if err := json.Unmarshal(body, &v); err != nil {
			return string(body), err
		}
		return v, nil
	}

	// Auto.
	switch {
	case mediaType == "application/json":
		if len(bytes.TrimSpace(body)) == 0 {
			return nil, nil
		}
		var v any
		if err := json.Unmarshal(body, &v); err != nil {
			return string(body), err
		}
		return v, nil
	case strings.HasPrefix(mediaType, "text/"):
		return string(body), nil
	default:
		return bytesToNumberArray(body), nil
	}
}

func (n *HTTPRequestNode) setStatus(fill, text string) {
	if n.status != nil {
		n.status(fill, text)
	}
}

// truncateStatus clips s to at most max characters, suffixed with "…".
// Used to keep status text compact.
func truncateStatus(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

// HTTPRequestTypeInfo returns the NodeTypeInfo for the http-request node.
func HTTPRequestTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "http-request",
		Category:    "network",
		Label:       "HTTP Request",
		Description: "Performs an outbound HTTP request and emits the response",
		Icon:        "mdi-cloud-search",
		Defaults: map[string]any{
			"method":          "GET",
			"url":             "",
			"responseFormat":  "auto",
			"bodyEncoding":    "auto",
			"timeout":         int(defaultRequestTimeout / time.Second),
			"followRedirects": true,
			"tlsInsecure":     false,
			"errorMode":       "passthrough",
			"headers":         map[string]any{},
		},
		Inputs:  1,
		Outputs: 1,
	}
}
