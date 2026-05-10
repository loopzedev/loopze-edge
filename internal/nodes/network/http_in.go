// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package network

import (
	"github.com/loopzedev/loopze-edge/internal/nodes"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// Body parse modes.
const (
	bodyParseAuto   = "auto"
	bodyParseString = "string"
	bodyParseJSON   = "json"
	bodyParseBuffer = "buffer"
	bodyParseNone   = "none"
)

const (
	defaultMaxBodyBytes    int64 = 1 << 20 // 1 MiB
	defaultResponseTimeout       = 30 * time.Second
)

// HTTPInNode exposes a flow-defined HTTP endpoint and emits one message
// per matching request. The handler blocks on the message's res handle
// until a paired http-response node (or the response-registry sweeper)
// completes the slot.
type HTTPInNode struct {
	cfg flow.NodeConfig

	nodes.BaseNode
	errorFn flow.ErrorFunc

	// Injected by the engine via HTTPMuxProvider.
	registry *flow.ResponseRegistry
	root     string

	// Parsed configuration.
	method          string
	path            string
	bodyParse       string
	maxBodyBytes    int64
	responseTimeout time.Duration
	cors            *corsConfig

	// statusMu guards the latest reported status text so that
	// OnHTTPRouteConflict can override the optimistic green from Start.
	statusMu sync.Mutex
	confTxt  string // non-empty when the node is in a conflict state
}

// corsConfig is the parsed CORS sub-section of the node configuration.
type corsConfig struct {
	origins     []string
	methods     []string
	headers     []string
	credentials bool
	maxAge      int
}

// NewHTTPInNode is the NodeFactory for the "http-in" node type.
func NewHTTPInNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &HTTPInNode{cfg: cfg}, nil
}

// Init validates and parses the configuration. Invalid configs return
// an error so the engine refuses the deploy for this node.
func (n *HTTPInNode) Init() error {
	props := n.cfg.Properties

	method := strings.ToUpper(strings.TrimSpace(nodes.StringVal(props, "method", "GET")))
	if method == "" {
		method = "GET"
	}
	if !isValidMethod(method) {
		return fmt.Errorf("http-in %s: invalid method %q", n.cfg.ID, method)
	}
	n.method = method

	path := strings.TrimSpace(nodes.StringVal(props, "path", ""))
	if path == "" || !strings.HasPrefix(path, "/") {
		return fmt.Errorf("http-in %s: path must start with '/'", n.cfg.ID)
	}
	n.path = path

	parseMode := strings.ToLower(nodes.StringVal(props, "bodyParse", bodyParseAuto))
	switch parseMode {
	case bodyParseAuto, bodyParseString, bodyParseJSON, bodyParseBuffer, bodyParseNone:
	default:
		return fmt.Errorf("http-in %s: invalid bodyParse %q", n.cfg.ID, parseMode)
	}
	n.bodyParse = parseMode

	maxBytes := nodes.IntVal(props, "maxBodyBytes", int(defaultMaxBodyBytes))
	if maxBytes < 0 {
		return fmt.Errorf("http-in %s: maxBodyBytes must be >= 0", n.cfg.ID)
	}
	n.maxBodyBytes = int64(maxBytes)

	timeoutSec := nodes.IntVal(props, "responseTimeout", int(defaultResponseTimeout/time.Second))
	if timeoutSec < 0 {
		return fmt.Errorf("http-in %s: responseTimeout must be >= 0", n.cfg.ID)
	}
	n.responseTimeout = time.Duration(timeoutSec) * time.Second

	if c := parseCORS(props); c != nil {
		n.cors = c
	}

	return nil
}

// SetError implements flow.ErrorProvider so the node can report async
// errors (e.g. body parse failures inside the HTTP handler) into the
// engine's standard error pipeline.
func (n *HTTPInNode) SetError(fn flow.ErrorFunc) { n.errorFn = fn }

// SetHTTPMux implements flow.HTTPMuxProvider.
func (n *HTTPInNode) SetHTTPMux(registry *flow.ResponseRegistry, root string) {
	n.registry = registry
	n.root = root
}

// HTTPRoutes implements flow.HTTPInProvider. Returns the configured
// route plus, when CORS is configured, an OPTIONS route that handles
// preflight without emitting a flow message.
func (n *HTTPInNode) HTTPRoutes() []flow.HTTPRouteSpec {
	specs := []flow.HTTPRouteSpec{{
		Method:  n.method,
		Path:    n.path,
		Handler: n.handle,
	}}
	if n.cors != nil && n.method != http.MethodOptions && n.method != "*" {
		specs = append(specs, flow.HTTPRouteSpec{
			Method:  http.MethodOptions,
			Path:    n.path,
			Handler: n.handlePreflight,
		})
	}
	return specs
}

// Start emits the optimistic "listening" status. The actual route
// registration happens in the engine's rebuildHTTPMux step which runs
// directly after; if registration conflicts, OnHTTPRouteConflict
// overrides this with red.
func (n *HTTPInNode) Start() error {
	if n.Send == nil {
		return fmt.Errorf("http-in %s: send not wired", n.cfg.ID)
	}
	if n.registry == nil {
		return fmt.Errorf("http-in %s: response registry not injected", n.cfg.ID)
	}
	n.statusMu.Lock()
	n.confTxt = ""
	n.statusMu.Unlock()
	n.setListeningStatus()
	slog.Info("http-in started",
		"node_id", n.cfg.ID, "method", n.method, "path", n.path,
		"body_parse", n.bodyParse, "max_body_bytes", n.maxBodyBytes,
	)
	return nil
}

// HandleMessage is a no-op. http-in is a source node; messages enter
// the flow exclusively through the HTTP handler.
func (n *HTTPInNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	return nil, nil
}

// Stop is a no-op. The handler closures live in the chi router; the
// engine drains the response registry on stop so any handler still
// blocked on <-done returns immediately with a 503.
func (n *HTTPInNode) Stop() error {
	slog.Info("http-in stopped", "node_id", n.cfg.ID)
	return nil
}

// OnHTTPRouteConflict implements flow.HTTPInConflictReporter.
func (n *HTTPInNode) OnHTTPRouteConflict(reason string) {
	n.statusMu.Lock()
	n.confTxt = reason
	n.statusMu.Unlock()
	if n.Status != nil {
		n.Status("red", "route conflict: "+reason)
	}
}

// setListeningStatus pushes the green idle status, formatted to include
// the full URL prefix ("listening · POST /endpoint/webhook").
func (n *HTTPInNode) setListeningStatus() {
	if n.Status == nil {
		return
	}
	n.statusMu.Lock()
	conflict := n.confTxt
	n.statusMu.Unlock()
	if conflict != "" {
		// A conflict was already reported; don't overwrite with green.
		return
	}
	full := n.path
	if n.root != "" {
		full = strings.TrimSuffix(n.root, "/") + n.path
	}
	n.Status("green", "listening · "+n.method+" "+full)
}

// ─── Request handling ───────────────────────────────────────────────────────

// handle is the chi handler bound to the configured (method, path).
// It parses the body, builds the msg.req envelope, registers a slot in
// the response registry, fires the message, and blocks on <-done.
func (n *HTTPInNode) handle(w http.ResponseWriter, r *http.Request) {
	if n.registry == nil {
		http.Error(w, "internal: response registry not initialised", http.StatusInternalServerError)
		return
	}

	// Pre-set CORS headers on the actual response (for non-preflight
	// requests). Headers in net/http are buffered until the first
	// WriteHeader/Write call by http-response.
	if n.cors != nil {
		n.applyCORSHeaders(w, r)
	}

	payload, parseErr := n.readBody(r)
	if parseErr != nil {
		// 413 vs 400 vs 415 — be specific.
		status := parseErr.statusCode
		if status == 0 {
			status = http.StatusBadRequest
		}
		http.Error(w, parseErr.publicMsg, status)
		if n.errorFn != nil {
			n.errorFn(parseErr.cause, nil)
		}
		return
	}

	// Register the response slot before sending into the flow so a
	// fast http-response node can resolve the handle immediately.
	handle, done := n.registry.Register(w, r, n.responseTimeout, n.cfg.ID, n.cfg.FlowID)

	msg := flow.NewMessage()
	if payload != nil {
		msg.SetPayload(payload)
	}
	msg.Set("req", buildReqEnvelope(r))
	msg.Set("res", handle)

	n.Send(0, msg)

	// Block until http-response writes the body, the sweeper expires
	// the slot (default 504), or the engine drains on shutdown (503).
	<-done
}

// handlePreflight answers a CORS preflight without emitting a flow
// message. Only registered when n.cors is non-nil.
func (n *HTTPInNode) handlePreflight(w http.ResponseWriter, r *http.Request) {
	if n.cors == nil {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	n.applyCORSHeaders(w, r)
	if v := r.Header.Get("Access-Control-Request-Headers"); v != "" && len(n.cors.headers) > 0 {
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(n.cors.headers, ", "))
	}
	if len(n.cors.methods) > 0 {
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(n.cors.methods, ", "))
	} else {
		w.Header().Set("Access-Control-Allow-Methods", n.method)
	}
	if n.cors.maxAge > 0 {
		w.Header().Set("Access-Control-Max-Age", strconv.Itoa(n.cors.maxAge))
	}
	w.WriteHeader(http.StatusNoContent)
}

// applyCORSHeaders writes the per-response CORS headers based on the
// request's Origin and the configured policy. Idempotent — safe to
// call from both the regular handler and the preflight handler.
func (n *HTTPInNode) applyCORSHeaders(w http.ResponseWriter, r *http.Request) {
	if n.cors == nil {
		return
	}
	origin := r.Header.Get("Origin")
	allowed := corsResolveAllowedOrigin(origin, n.cors.origins)
	if allowed == "" {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", allowed)
	if allowed != "*" {
		w.Header().Add("Vary", "Origin")
	}
	if n.cors.credentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
}

// ─── Body parsing ───────────────────────────────────────────────────────────

// bodyParseError carries enough context for the handler to send the
// right HTTP status and to forward the underlying cause to errorFn.
type bodyParseError struct {
	statusCode int
	publicMsg  string
	cause      error
}

func (n *HTTPInNode) readBody(r *http.Request) (any, *bodyParseError) {
	if n.bodyParse == bodyParseNone {
		return nil, nil
	}
	if r.Body == nil {
		return nil, nil
	}

	body := r.Body
	if n.maxBodyBytes > 0 {
		body = http.MaxBytesReader(nil, r.Body, n.maxBodyBytes)
	}
	defer body.Close()

	mediaType := mediaTypeOf(r.Header.Get("Content-Type"))

	switch n.bodyParse {
	case bodyParseString:
		return readToString(body)
	case bodyParseJSON:
		return parseJSONBody(body)
	case bodyParseBuffer:
		return readToNumberArray(body)
	}

	// Auto: dispatch by Content-Type.
	switch {
	case mediaType == "application/json":
		return parseJSONBody(body)
	case mediaType == "application/x-www-form-urlencoded":
		return parseFormBody(r, body, n.maxBodyBytes)
	case strings.HasPrefix(mediaType, "multipart/form-data"):
		return parseMultipartBody(r, body, n.maxBodyBytes)
	case strings.HasPrefix(mediaType, "text/"):
		return readToString(body)
	default:
		return readToNumberArray(body)
	}
}

func readToString(body io.Reader) (any, *bodyParseError) {
	b, err := io.ReadAll(body)
	if err != nil {
		return nil, classifyReadErr(err)
	}
	return string(b), nil
}

func readToNumberArray(body io.Reader) (any, *bodyParseError) {
	b, err := io.ReadAll(body)
	if err != nil {
		return nil, classifyReadErr(err)
	}
	return bytesToNumberArray(b), nil
}

func parseJSONBody(body io.Reader) (any, *bodyParseError) {
	b, err := io.ReadAll(body)
	if err != nil {
		return nil, classifyReadErr(err)
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, &bodyParseError{
			statusCode: http.StatusBadRequest,
			publicMsg:  "invalid JSON body",
			cause:      fmt.Errorf("http-in: json parse: %w", err),
		}
	}
	return v, nil
}

func parseFormBody(r *http.Request, body io.ReadCloser, maxBytes int64) (any, *bodyParseError) {
	// Replace r.Body with the size-limited reader so r.ParseForm uses it.
	r.Body = body
	if err := r.ParseForm(); err != nil {
		return nil, classifyReadErr(err)
	}
	out := make(map[string]string, len(r.PostForm))
	for k, vs := range r.PostForm {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out, nil
}

func parseMultipartBody(r *http.Request, body io.ReadCloser, maxBytes int64) (any, *bodyParseError) {
	r.Body = body
	memLimit := maxBytes
	if memLimit <= 0 || memLimit > 32<<20 {
		memLimit = 32 << 20 // 32 MiB cap on in-memory parts
	}
	if err := r.ParseMultipartForm(memLimit); err != nil {
		return nil, classifyReadErr(err)
	}

	out := make(map[string]any)
	if r.MultipartForm == nil {
		return out, nil
	}
	for k, vs := range r.MultipartForm.Value {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	for k, headers := range r.MultipartForm.File {
		if len(headers) == 0 {
			continue
		}
		fh := headers[0]
		f, err := fh.Open()
		if err != nil {
			return nil, classifyReadErr(err)
		}
		buf, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			return nil, classifyReadErr(err)
		}
		ct := fh.Header.Get("Content-Type")
		out[k] = map[string]any{
			"filename":    fh.Filename,
			"contentType": ct,
			"size":        fh.Size,
			"data":        bytesToNumberArray(buf),
		}
	}
	return out, nil
}

// classifyReadErr maps a generic io / net/http read error to either a
// 413 (body too large) or a 400 (bad request) parse error.
func classifyReadErr(err error) *bodyParseError {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return &bodyParseError{
			statusCode: http.StatusRequestEntityTooLarge,
			publicMsg:  "request body too large",
			cause:      fmt.Errorf("http-in: body too large: %w", err),
		}
	}
	return &bodyParseError{
		statusCode: http.StatusBadRequest,
		publicMsg:  "could not read request body",
		cause:      fmt.Errorf("http-in: read: %w", err),
	}
}

// bytesToNumberArray converts raw bytes to a []int so the value
// survives JSON round-trips without becoming base64. Mirrors the
// convention used by the MQTT nodes.
func bytesToNumberArray(b []byte) []int {
	out := make([]int, len(b))
	for i, x := range b {
		out[i] = int(x)
	}
	return out
}

// ─── Request envelope ───────────────────────────────────────────────────────

// buildReqEnvelope flattens the most useful pieces of *http.Request
// into a plain map. Keys are stable and lowercase to ease downstream
// matching in Switch / Change nodes.
func buildReqEnvelope(r *http.Request) map[string]any {
	headers := make(map[string]string, len(r.Header))
	for k, v := range r.Header {
		if len(v) == 0 {
			continue
		}
		headers[strings.ToLower(k)] = v[0]
	}
	cookies := make(map[string]string)
	for _, c := range r.Cookies() {
		cookies[c.Name] = c.Value
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if v := r.Header.Get("X-Forwarded-Proto"); v != "" {
		scheme = strings.ToLower(strings.SplitN(v, ",", 2)[0])
	}

	return map[string]any{
		"method":     r.Method,
		"url":        r.RequestURI,
		"path":       r.URL.Path,
		"params":     extractURLParams(r),
		"query":      r.URL.Query(),
		"headers":    headers,
		"remoteAddr": r.RemoteAddr,
		"host":       r.Host,
		"scheme":     scheme,
		"cookies":    cookies,
	}
}

// extractURLParams pulls chi's named URL params (":id" etc.) out of
// the request context. Empty when no chi context is attached.
func extractURLParams(r *http.Request) map[string]string {
	out := make(map[string]string)
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return out
	}
	for i, k := range rctx.URLParams.Keys {
		if i >= len(rctx.URLParams.Values) {
			break
		}
		if k == "*" {
			continue
		}
		out[k] = rctx.URLParams.Values[i]
	}
	return out
}

// ─── Helpers ───────────────────────────────────────────────────────────────

func isValidMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions, "*":
		return true
	}
	return false
}

func mediaTypeOf(contentType string) string {
	if i := strings.Index(contentType, ";"); i >= 0 {
		return strings.ToLower(strings.TrimSpace(contentType[:i]))
	}
	return strings.ToLower(strings.TrimSpace(contentType))
}

func parseCORS(props map[string]any) *corsConfig {
	raw, ok := props["cors"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	c := &corsConfig{}
	c.origins = stringSlice(raw, "origins")
	c.methods = stringSlice(raw, "methods")
	c.headers = stringSlice(raw, "headers")
	if v, ok := raw["credentials"].(bool); ok {
		c.credentials = v
	}
	c.maxAge = nodes.IntVal(raw, "maxAge", 600)
	if len(c.origins) == 0 {
		return nil
	}
	return c
}

func stringSlice(m map[string]any, key string) []string {
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// corsResolveAllowedOrigin returns the value to put in
// Access-Control-Allow-Origin. Returns "" when the request's Origin
// is not on the allowlist (the browser then enforces same-origin).
func corsResolveAllowedOrigin(origin string, allowList []string) string {
	if origin == "" {
		return ""
	}
	for _, allowed := range allowList {
		if allowed == "*" {
			return "*"
		}
		if strings.EqualFold(allowed, origin) {
			return origin
		}
	}
	return ""
}

// HTTPInTypeInfo returns the NodeTypeInfo for registering the http-in node.
func HTTPInTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "http-in",
		Category:    "network",
		Label:       "HTTP In",
		Description: "Exposes an HTTP endpoint and emits a message per request",
		Icon:        "mdi-cloud-download",
		Defaults: map[string]any{
			"method":          "GET",
			"path":            "/endpoint",
			"bodyParse":       "auto",
			"maxBodyBytes":    int(defaultMaxBodyBytes),
			"responseTimeout": int(defaultResponseTimeout / time.Second),
		},
		Inputs:  0,
		Outputs: 1,
	}
}
