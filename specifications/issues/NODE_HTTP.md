# Issue: HTTP Nodes – Receive, Response & Request

## Status: Open

## Problem Description

LOOPZE needs a complete HTTP node trio so that flows can both **expose
HTTP endpoints** (server side) and **call external HTTP APIs** (client
side). Three new node types are introduced:

- `http-in` – **HTTP Receive** — exposes an HTTP endpoint, emits one
  message per incoming request.
- `http-response` — sends the HTTP response back to the original
  caller. Paired with `http-in` via the request handle on the message.
- `http-request` — performs an outbound HTTP request and emits the
  response as a message.

Together they allow LOOPZE to act as an HTTP server (webhooks,
integrations, REST-like endpoints inside flows) and as an HTTP client
(polling external APIs, sending data to remote services).

The Node-RED nodes `http in`, `http response`, and `http request` are
the conceptual template; the wire format and the `msg.req` / `msg.res`
handles are deliberately kept compatible in spirit.

## Overview

| Node Type | Type ID | Canvas Inputs | Canvas Outputs | Description |
|---|---|---|---|---|
| **HTTP Receive** | `http-in` | 0 | 1 | Listens on a configured method+path; emits one message per incoming HTTP request |
| **HTTP Response** | `http-response` | 1 | 0 | Sends the HTTP response back to the requester, using the request handle on the incoming message |
| **HTTP Request** | `http-request` | 1 | 1 | Performs an outbound HTTP request and emits the response |

### Typical Flow Shapes

**Server side – webhook receiver:**

```
[http-in  POST /webhook] → [Function: parse/validate] → [http-response  200]
                                       ↓
                                 [Debug / MQTT-out / …]
```

**Client side – external API call:**

```
[Inject 30s] → [Change: build req] → [http-request GET …] → [JSON Parser] → [Debug]
```

**Combined – proxy / transform:**

```
[http-in GET /products/:id] → [http-request GET https://api.example.com/p/{{id}}]
                                                         ↓
                                              [Change: shape body]
                                                         ↓
                                                 [http-response]
```

## Requirements

### 1. HTTP Receive Node (`http-in`)

- **Canvas**: 0 inputs, 1 output (source node)
- **Function**: Registers an HTTP route on the LOOPZE-internal *flow
  endpoint mux* (see § 4). For every matching request a message with
  `msg.req` and `msg.res` is emitted on the output. The HTTP response
  is sent back via a paired `http-response` (or, as fallback, with a
  default empty 200 — see § 5).

- **Configuration**:
  - `method` (string) — `GET` (default), `POST`, `PUT`, `PATCH`,
    `DELETE`, `HEAD`, `OPTIONS`, or `*` (any)
  - `path` (string) — path relative to the flow-endpoint prefix,
    e.g. `/webhook` or `/devices/:id/state`. Chi-style URL params
    (`:name`, `*` catch-all) are supported. Must start with `/`
  - `bodyParse` (string) — how `msg.payload` is populated from the
    request body:
    - `auto` (default) — by Content-Type:
      - `application/json` → JSON-parsed (`map`/`[]any`/scalar)
      - `application/x-www-form-urlencoded` → `map[string]string`
      - `multipart/form-data` → `map[string]any` (text fields as
        string, file fields as `{filename, contentType, size, data}`
        with `data` as number array — see § 7)
      - `text/*` → string
      - everything else → number array (raw bytes)
    - `string` — force string (UTF-8 decoded bytes)
    - `json` — force JSON parse; on error → 400 to the caller and
      `_error` on the flow path (catchable)
    - `buffer` — force number array (raw bytes)
    - `none` — body is not read at all; `msg.payload` stays unset
  - `maxBodyBytes` (number) — request body size limit, default
    `1048576` (1 MiB). Exceeding it → 413 to the caller, no flow
    message emitted
  - `responseTimeout` (number, seconds) — how long the engine waits
    for a paired `http-response` before sending the **fallback
    response** (see § 5). Default: 30. `0` disables the timeout
    (handler waits until the request context is cancelled — risky;
    only for hand-managed flows)
  - `cors` (object, optional) — CORS configuration; if set, the
    server emits the appropriate `Access-Control-*` headers and
    answers `OPTIONS` preflights itself (in that case the
    preflight does **not** emit a flow message):
    - `origins` (string[]) — list of allowed origins or `["*"]`
    - `methods` (string[]) — defaults to the configured `method`
    - `headers` (string[]) — allowed request headers
    - `credentials` (boolean) — default `false`
    - `maxAge` (number, seconds) — default `600`

- **Outgoing message** (one per request):
  ```json
  {
    "_msgid": "...",
    "payload": "<parsed body — depends on bodyParse>",
    "req": {
      "method": "POST",
      "url": "/devices/42/state?force=1",
      "path": "/devices/42/state",
      "params": { "id": "42" },
      "query":  { "force": ["1"] },
      "headers": { "content-type": "application/json", "...": "..." },
      "remoteAddr": "192.0.2.10:53412",
      "host": "loopze.local",
      "scheme": "https",
      "cookies": { "session": "abc" }
    },
    "res": "<opaque response handle — see § 4>"
  }
  ```
  - Header keys are lowercased to ease downstream matching.
  - `query` is `map[string][]string` to preserve repeated keys.
  - `params` only contains URL parameters declared in `path`.
  - `res` is an opaque token (string ID + internal pointer) that
    only the engine can resolve; it is omitted from JSON serialization
    and flagged as non-cloneable so a Debug node prints `<response
    handle>` instead of the underlying object.

- **Status display**:
  - Green: `listening · METHOD /path` once the route is registered
  - Red: route registration failed (e.g., conflict with another
    `http-in` on the same `method+path`) — error message in status
  - Pulse blue (briefly) on each incoming request — analogous to MQTT
    in (optional, can be deferred)

- **Properties panel**:

```
┌──────────────────────────────────────────────┐
│  HTTP Receive                                 │
├──────────────────────────────────────────────┤
│                                               │
│  Method                                       │
│  ┌────────────────────────────────────────┐   │
│  │ POST                              ▼   │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Path                                         │
│  ┌────────────────────────────────────────┐   │
│  │ /webhook                              │   │
│  └────────────────────────────────────────┘   │
│  ℹ Full URL: https://host/endpoint/webhook   │
│                                               │
│  Parse body as                                │
│  ┌────────────────────────────────────────┐   │
│  │ Auto (by Content-Type)            ▼   │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Max body size (bytes)        Response wait   │
│  ┌──────────────┐             ┌──────────┐    │
│  │   1048576     │            │    30    │ s  │
│  └──────────────┘             └──────────┘    │
│                                               │
│  ▼ CORS (optional)                            │
│  Origins:  [* or comma-list                ]  │
│  Headers:  [content-type,authorization     ]  │
│  ☐ Allow credentials   Max-Age: [600] s       │
│                                               │
└──────────────────────────────────────────────┘
```

### 2. HTTP Response Node (`http-response`)

- **Canvas**: 1 input, 0 outputs (sink node)
- **Function**: Sends the HTTP response for the request that produced
  the inbound message. Looks up the request via `msg.res`. Without a
  valid `msg.res` the node sets the catchable error
  `"http-response: no request handle on msg.res"` and discards the
  message.

- **Configuration**:
  - `statusCode` (number, optional) — default status if `msg.statusCode`
    is missing. Default: `200`
  - `headers` (object, optional) — static headers merged with
    `msg.headers` (msg keys win on conflict)

- **Incoming message**:
  - `msg.payload` — body to send. Encoded by type:
    - `string` → sent as-is, default `Content-Type: text/plain;
      charset=utf-8` (only if no Content-Type is set elsewhere)
    - `[]byte` / number array → raw bytes, default
      `application/octet-stream`
    - `map` / `slice` → `json.Marshal`, default `application/json`
    - `nil` / missing → empty body
  - `msg.statusCode` (number, optional) — overrides the configured
    status
  - `msg.headers` (object, optional) — string-to-string map; merged
    with `headers` from the config (msg wins)
  - `msg.cookies` (object, optional) — cookies to set, e.g.
    `{ "session": { "value": "abc", "maxAge": 3600, "path": "/",
    "httpOnly": true, "secure": true, "sameSite": "lax" } }`. A bare
    string value `{ "session": "abc" }` is shorthand for "value only,
    no extra attributes"

- A response can be sent **only once** per `msg.res`. Subsequent
  `http-response` invocations on the same handle are no-ops (with a
  catchable warning `"http-response: already sent"`). This guards
  against fan-out where multiple branches all terminate in
  `http-response`.

- **Properties panel**:

```
┌──────────────────────────────────────────────┐
│  HTTP Response                                │
├──────────────────────────────────────────────┤
│                                               │
│  Status code                                  │
│  ┌────────────────────────────────────────┐   │
│  │ 200                                   │   │
│  └────────────────────────────────────────┘   │
│  ℹ Overridden by msg.statusCode               │
│                                               │
│  Headers (static)                             │
│  ┌──────────────┐ ┌──────────────┐ ┌───┐     │
│  │ X-Foo         │ │ bar          │ │ × │     │
│  └──────────────┘ └──────────────┘ └───┘     │
│  [+ Add header]                               │
│                                               │
│  ℹ msg.headers / msg.cookies override or       │
│    extend these per message.                   │
└──────────────────────────────────────────────┘
```

### 3. HTTP Request Node (`http-request`)

- **Canvas**: 1 input, 1 output
- **Function**: Performs an HTTP request to the configured URL (or
  `msg.url`) and emits the response on the output.

- **Configuration**:
  - `method` (string) — `GET` (default), `POST`, `PUT`, `PATCH`,
    `DELETE`, `HEAD`, `OPTIONS`, or `use msg.method`
  - `url` (string, optional) — request URL. Supports
    `{{mustache}}` substitutions over `msg` (analogous to the
    Template node — same engine reused). If empty, `msg.url` is
    required
  - `responseFormat` (string) — how the response body is converted
    into `msg.payload`:
    - `auto` (default) — by `Content-Type`:
      `application/json` → JSON-parsed, `text/*` → string,
      everything else → number array
    - `string` — UTF-8 decoded string
    - `json` — JSON-parsed; on error sets `msg.parseError` and falls
      back to `string`
    - `buffer` — number array (raw bytes)
  - `bodyEncoding` (string) — how `msg.payload` is encoded into the
    request body:
    - `auto` (default) — `string` → as-is (no Content-Type added);
      `map`/`slice` → JSON + `Content-Type: application/json`;
      number array / `[]byte` → raw bytes; `nil` → no body
    - `json` — force JSON encoding
    - `form` — encode `map[string]any` as
      `application/x-www-form-urlencoded`
    - `text` — coerce to string
    - `none` — no body (even for non-GET)
  - `headers` (object) — static headers; merged with `msg.headers`
    (msg wins)
  - `query` (object, optional) — static query params merged with
    parsed query of `url`; `msg.query` (object or array of `[k,v]`
    tuples) wins on conflict
  - `auth` (object, optional) — outgoing authentication:
    - `type`: `none` (default), `basic`, `bearer`
    - `username` / `password` (basic) — strings
    - `token` (bearer) — string
  - `timeout` (number, seconds) — request timeout. Default: 30
  - `followRedirects` (boolean) — default `true` (Go's default
    follows up to 10 redirects)
  - `tlsInsecure` (boolean) — disable TLS certificate verification.
    Default: `false`. Logged at WARN level on every deploy when
    enabled
  - `errorMode` (string) — how non-2xx responses are handled:
    - `passthrough` (default) — emit the response as a regular
      message; downstream Switch decides
    - `error` — non-2xx triggers a catchable error and **no** message
      goes out the output

- **Incoming message** (all optional, override config):
  - `msg.method`, `msg.url`, `msg.headers`, `msg.query`
  - `msg.payload` (request body — encoded per `bodyEncoding`)
  - `msg.timeout` (number, seconds) — per-request override
  - `msg.followRedirects` (boolean)

- **Outgoing message**:
  ```json
  {
    "statusCode": 200,
    "headers": { "content-type": "application/json", "...": "..." },
    "responseUrl": "https://api.example.com/p/42",
    "redirectList": [
      { "location": "https://...", "status": 302, "cookies": {...} }
    ],
    "payload": "<parsed body — depends on responseFormat>"
  }
  ```
  - `responseUrl` is the final URL after redirects.
  - `redirectList` is only present when at least one redirect was
    followed. The pre-existing `msg` properties are preserved unless
    overwritten by these fields (so e.g. a correlation token can be
    carried through).

- **Status display**:
  - Pulse blue while a request is in flight (best-effort indicator)
  - Green: idle (recent success) — format `200 · 142ms`
  - Yellow: timeout / network error (transient)
  - Red: configuration error (e.g., invalid URL template)

- **Properties panel**:

```
┌──────────────────────────────────────────────┐
│  HTTP Request                                 │
├──────────────────────────────────────────────┤
│                                               │
│  Method                                       │
│  ┌────────────────────────────────────────┐   │
│  │ GET                               ▼   │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  URL                                          │
│  ┌────────────────────────────────────────┐   │
│  │ https://api.example.com/p/{{payload.id}} │   │
│  └────────────────────────────────────────┘   │
│  ℹ {{mustache}} substitution over msg.        │
│                                               │
│  Return                                       │
│  ┌────────────────────────────────────────┐   │
│  │ Auto (by Content-Type)            ▼   │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Body encoding                                │
│  ┌────────────────────────────────────────┐   │
│  │ Auto                              ▼   │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Timeout: [30] s   Follow redirects: ☑        │
│  ☐ Insecure TLS (skip verify)  ⚠              │
│                                               │
│  ▼ Authentication                             │
│  Type: ( ) None  ( • ) Basic  ( ) Bearer      │
│  Username: [........................]         │
│  Password: [••••••••................]         │
│                                               │
│  ▼ Headers                                    │
│  ┌──────────────┐ ┌──────────────┐ ┌───┐     │
│  │ Accept        │ │ application/.. │ │ × │   │
│  └──────────────┘ └──────────────┘ └───┘     │
│  [+ Add header]                               │
│                                               │
│  Non-2xx response                             │
│  ( • ) Pass through    ( ) Treat as error     │
│                                               │
└──────────────────────────────────────────────┘
```

### 4. Flow Endpoint Mux (server-side integration)

The HTTP-In nodes need their routes mounted on the same listener as
the LOOPZE web UI but **strictly separated** from the management
routes:

- **Mount prefix**: `/endpoint` (default), configurable via env
  `LOOPZE_HTTP_NODE_ROOT` and a CLI flag. The prefix is appended
  *after* `BasePath` if set, so the full URL is
  `<BasePath>/endpoint/<user-path>`
- **Bypass**:
  - **No** auth middleware (`authMW.Authenticate`) — flow endpoints
    are public by default; the user is responsible for auth inside
    the flow (e.g., a Function node checking `msg.req.headers.authorization`)
  - **No** CSRF middleware — CSRF is a browser-form concern, not
    appropriate for webhook receivers
  - Recoverer, RequestID, structured logging, real-IP middleware
    **do** still apply
- **Dynamic routing**: Routes are registered per `http-in` node on
  deploy and unregistered on stop / re-deploy. Implemented via a
  single chi sub-router that is **rebuilt** on every deploy
  (write-once, swap-pointer pattern — no concurrent route table
  mutation):
  ```
  /endpoint/* → atomic.Pointer[chi.Router]   ← swapped on each deploy
  ```
- **Route conflicts**: Two `http-in` nodes with the same `method`
  and `path` ⇒ both go red, deploy proceeds for the rest. Conflict
  is reported as a deploy diagnostic, not a hard failure (analogous
  to how the engine handles other config errors)
- **`msg.res` lifecycle**: When an HTTP-In handler is invoked it
  registers an entry `{handleID → (responseWriter, doneCh,
  expiresAt)}` in a per-engine `responseRegistry`. The flow message
  carries `handleID` as an opaque string. `http-response` looks up
  the handle, writes the response, marks it done, and removes it
  from the registry. A background sweeper expires entries whose
  `responseTimeout` has passed and sends a default `504 Gateway
  Timeout` (configurable to `200` empty if the user wants
  fire-and-forget). If a handle ID arrives that is unknown (e.g.
  after redeploy), `http-response` raises a catchable error

### 5. Default Response Behaviour (no `http-response` wired)

If the flow ends without ever calling `http-response`, the request
context blocks the HTTP handler. The behaviour is governed by
`responseTimeout` on the `http-in` node:

- After `responseTimeout` seconds: server sends `504 Gateway Timeout`
  with body `{"error":"flow did not respond"}` and an `_error`
  message is dispatched on the catch path of the `http-in` node
- `responseTimeout = 0`: the handler blocks until the **request
  context** is cancelled (client disconnect or server shutdown).
  Suitable for fire-and-forget webhooks where the client doesn't care
  about the body — but the user must understand the lifecycle
- The timeout is independent of any `http-request` timeouts further
  inside the flow

This deliberately differs from Node-RED, which silently sends a 200
when no response node is wired. We prefer a loud failure mode so
forgotten `http-response` wiring is detected during development.

### 6. Error Handling & Catch Integration

All three nodes are good Catch citizens (per the `Catch Node` issue):

- `http-in`: errors during route registration (path conflict, invalid
  pattern) ⇒ `errorFn(err, nil)` from `OnInit`. Body parse errors
  (per `bodyParse`) ⇒ caller gets 400 + catchable error
- `http-response`: missing `msg.res`, response already sent, write
  error after client disconnect ⇒ catchable error. The message itself
  is not forwarded (sink node)
- `http-request`: network error, timeout, TLS error, redirect loop,
  body decode error ⇒ catchable error. With `errorMode = error` a
  non-2xx status also triggers; with `passthrough` it does not
- All errors carry `_error.source.{id, type, name, flowId}` per the
  Catch contract

### 7. Body Encoding for Binary Data

`http-in` and `http-request` both produce/consume binary data
(uploaded files, image payloads, …). The convention follows the MQTT
nodes (see `NODE_MQTT.md`):

- **Read-side** (request body, response body): non-text bytes are
  delivered as a **number array** (`[]int`), e.g. `[222, 173, 190,
  239]`. Reason: Go's `encoding/json` serializes `[]byte` as base64,
  which is unreadable in the debug viewer
- **Write-side** (response body, request body): both `[]byte` and
  `[]int` (number array) are accepted and reconstructed to raw bytes
- **Multipart file fields**: each file is delivered as
  `{ "filename": "...", "contentType": "...", "size": n, "data":
  [<bytes>] }`. Files larger than `maxBodyBytes` cause a 413 before
  any flow message is emitted

## Data Structure

### workspace.json

```json
{
  "flows": [
    {
      "id": "flow-1",
      "type": "tab",
      "label": "API",
      "nodes": [
        {
          "id": "node-http-in-1",
          "type": "http-in",
          "name": "Receive Webhook",
          "x": 200, "y": 150, "z": "flow-1",
          "inputs": 0, "outputs": 1,
          "wires": [["node-fn-1"]],
          "config": {
            "method": "POST",
            "path": "/webhook",
            "bodyParse": "auto",
            "maxBodyBytes": 1048576,
            "responseTimeout": 30
          }
        },
        {
          "id": "node-http-resp-1",
          "type": "http-response",
          "name": "OK",
          "x": 700, "y": 150, "z": "flow-1",
          "inputs": 1, "outputs": 0,
          "wires": [],
          "config": {
            "statusCode": 200,
            "headers": { "x-handled-by": "loopze" }
          }
        },
        {
          "id": "node-http-req-1",
          "type": "http-request",
          "name": "Fetch product",
          "x": 200, "y": 350, "z": "flow-1",
          "inputs": 1, "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "method": "GET",
            "url": "https://api.example.com/p/{{payload.id}}",
            "responseFormat": "auto",
            "bodyEncoding": "auto",
            "timeout": 30,
            "followRedirects": true,
            "tlsInsecure": false,
            "headers": { "Accept": "application/json" },
            "auth": { "type": "bearer", "token": "..." },
            "errorMode": "passthrough"
          }
        }
      ]
    }
  ]
}
```

`http-in`, `http-response`, and `http-request` do **not** introduce
new config nodes for v1 — credentials live inline on the node. A
shared HTTP auth config node is a candidate for a follow-up issue
once we have multiple nodes that need to share an OAuth client or a
TLS bundle.

## Affected Files

### Backend – New Files

- `internal/nodes/http_in.go` — HTTP Receive: registers the route on
  deploy via the engine-provided `HTTPMuxProvider`, builds the
  `msg.req` / `msg.res` envelope, hands it off to `SendFunc`
- `internal/nodes/http_response.go` — HTTP Response: resolves the
  handle on `msg.res`, writes status / headers / body, marks the
  handle as completed
- `internal/nodes/http_request.go` — HTTP Request: builds and runs
  the outbound request via a shared `*http.Client`, parses the
  response per `responseFormat`
- `internal/nodes/http_in_test.go`, `http_response_test.go`,
  `http_request_test.go` — table-driven unit tests with
  `httptest.Server` for the request side and a fake mux + recorder
  for the receive side
- `internal/server/flow_endpoint.go` — flow endpoint mux:
  `atomic.Pointer[chi.Router]`, deploy-time rebuild, `responseRegistry`
  for handle bookkeeping, sweeper goroutine for response timeouts

### Backend – Adjustments

- `internal/server/server.go` —
  - mount `/endpoint/*` (configurable prefix) before the `/api/v1`
    routes; ensure `auth.CSRF()` and `authMW.Authenticate` are
    **not** in this branch
  - inject the flow-endpoint mux + response registry into the
    `flow.Engine` so the nodes can access them via a provider
    interface
- `internal/server/server.go: registerNodes()` — register
  `http-in`, `http-response`, `http-request`
- `internal/flow/registry.go` — new provider:
  ```go
  type HTTPMuxProvider interface {
      SetHTTPMux(mux HTTPMux)
  }
  type HTTPMux interface {
      Handle(method, path string, h http.HandlerFunc) (unregister func(), err error)
      ResponseRegistry() ResponseRegistry
  }
  type ResponseRegistry interface {
      Register(w http.ResponseWriter, r *http.Request, timeout time.Duration) (handleID string, done <-chan struct{})
      Resolve(handleID string) (ResponseSlot, bool)
  }
  ```
- `internal/flow/engine.go` —
  - new lifecycle hook: after the config-node start phase but before
    regular `Start()`, the engine builds a fresh chi sub-router and
    swaps it via `atomic.Pointer` so old routes are released
    atomically
  - on `Stop()`: cancel any outstanding response slots with a 503 and
    drain the sweeper goroutine
- `internal/config/config.go` — new field `HTTPNodeRoot` (default
  `/endpoint`), env `LOOPZE_HTTP_NODE_ROOT`. CLI flag
  `--http-node-root` mirrored from the env var

### Frontend – New Files

- `frontend/src/components/config/HttpInConfig.vue` — method dropdown,
  path input with full-URL preview, body parse mode, max body size,
  response wait, CORS sub-section
- `frontend/src/components/config/HttpResponseConfig.vue` — status
  code, static headers list, hint about `msg.statusCode` /
  `msg.headers` / `msg.cookies` overrides
- `frontend/src/components/config/HttpRequestConfig.vue` — method,
  URL with mustache hint, return type, body encoding, timeout,
  redirect/TLS flags, auth section, headers list, error mode
- `frontend/src/components/nodes/HttpInNode.vue` — input-anchor-less
  source node, body shows `METHOD /path`
- `frontend/src/components/nodes/HttpResponseNode.vue` — sink node,
  body shows status code
- `frontend/src/components/nodes/HttpRequestNode.vue` — body shows
  `METHOD <host>` of the configured URL

### Frontend – Adjustments

- `frontend/src/components/config/configEditors.ts` — dispatch
  `http-in` → `HttpInConfig`, `http-response` → `HttpResponseConfig`,
  `http-request` → `HttpRequestConfig`
- `frontend/src/components/nodes/tokens.ts` — palette tokens:
  `http-in` (input, green), `http-response` (output, orange),
  `http-request` (function, blue) — group `network` (new) or
  `connector` (existing — pick whichever is consistent with MQTT)
- `frontend/src/types/flow.ts` — add `http-in`, `http-response`,
  `http-request` to the `NodeType` union
- `frontend/src/components/help/docs.ts` — three help entries with
  `msg.req` / `msg.res` schema and concrete examples (webhook,
  proxy, polling)
- `frontend/src/components/PropertyPanel.vue` — already dispatches
  via `configEditors.ts`; no change beyond registering the three new
  configs

### Go Dependencies

- Standard library `net/http` is sufficient. **No** new third-party
  client (e.g., `resty`) — keeps the dependency footprint small and
  matches how MQTT/Modbus/OPC-UA each carry exactly the one library
  they need
- `github.com/go-chi/chi/v5` — already vendored; reused for the
  flow-endpoint sub-router

## Technical Notes

### `msg.res` Handle — Why an Indirection

The naive design would put `*http.ResponseWriter` directly on the
message. That fails for three reasons:

1. **Concurrency**: messages may fan out / clone on the way; multiple
   branches must not race on the same writer
2. **Serializability**: the Debug node and the message clone path
   `json.Marshal` parts of the message — a `ResponseWriter` cannot be
   marshalled
3. **Cross-flow safety**: a handle that crosses into Link-Out → Link-In
   to a different flow stays valid because the registry is engine-wide

The opaque handle ID is a string; the registry is the single source
of truth for the writer pointer. The ID is non-cloneable
(`COWClone()` carries it by reference, never by deep-copy) and is
flagged so JSON marshalling renders it as `"<response handle>"`.

### Response Registry — Concurrency

```go
type ResponseSlot struct {
    w        http.ResponseWriter
    r        *http.Request
    done     chan struct{}
    once     sync.Once   // ensures Write() is called at most once
    deadline time.Time
}
```

- `Resolve` returns a `(slot, ok)` pair; the caller drives `slot.once`
- The sweeper runs every second and closes any slot whose deadline
  has passed (writes the configured fallback response)
- `engine.Stop()` closes all open slots with `503 Service
  Unavailable`

### Route Mux Rebuild on Deploy

```go
// engine.deploy()
newMux := chi.NewRouter()
for _, n := range httpInNodes { n.RegisterOn(newMux) }
e.muxPtr.Store(newMux)   // atomic swap; old in-flight requests
                          // continue on the captured pointer
```

The chi router is immutable after construction in our usage, so we
build a fresh one each deploy and swap. In-flight handlers keep
running on the old router until their handler returns. The serving
wrapper resolves the current router on every request:

```go
http.HandleFunc("/endpoint/*", func(w, r) {
    e.muxPtr.Load().ServeHTTP(w, r)
})
```

### URL Template (`{{mustache}}`) in `http-request`

The same template engine the Template node uses is reused
(`internal/nodes/template.go`). Resolution context is `msg`. Failed
lookups render as empty string with a warning log — same semantics
as the Template node.

### TLS Insecure — Loud Warning

`tlsInsecure: true` produces a deploy-time WARN log:

```
WARN  http-request: TLS verification disabled
      node=node-http-req-1 url=https://insecure.example.com/...
```

…repeated on every deploy. We intentionally do **not** add a UI
banner — log is enough; users who set this flag knew what they were
doing.

### CORS Preflight Handling

When `cors.origins` is configured, the registered route also accepts
`OPTIONS` and answers preflight requests **without** emitting a flow
message. This is mandatory: a flow that does its own
`http-response` for `OPTIONS` is fragile and confusing. The CORS
config covers the 95 % case; for full custom preflight logic, the
user can leave `cors` empty and add an explicit `http-in METHOD=OPTIONS`
themselves.

### Limits & Timeouts

| Limit | Default | Reason |
|---|---|---|
| `maxBodyBytes` (per `http-in`) | 1 MiB | OWASP-style default; covers 99 % of webhooks; prevents memory blow-up |
| `responseTimeout` (per `http-in`) | 30 s | Matches Go's default `WriteTimeout` of 60 s with margin |
| `timeout` (per `http-request`) | 30 s | Sane upstream default; can be overridden per-message |
| Redirect chain (`http-request`) | 10 | Go `http.Client` default |
| In-flight response slots | unbounded | Bounded transitively by the OS / Go HTTP server's connection limits — not a separate ceiling in v1 |

### Path Conflicts vs. Static Frontend

The flow-endpoint mux lives under `/endpoint/*` and cannot collide
with `/api/v1/*`, `/ws`, `/login`, `/logout`, `/health`, or the SPA
fallback. The configurable `LOOPZE_HTTP_NODE_ROOT` allows operators
to place flow endpoints at e.g. `/hooks` or `/public-api`, but
**not** at `/` (validated on startup — empty / `/` is rejected with
a clear error). This matches the threat model: flow endpoints are
attacker-influenced, the management surface is not.

## Tests

| Test | Verifies |
|---|---|
| `TestHttpIn_RegistersRoute` | After deploy the route is reachable; before deploy it 404s |
| `TestHttpIn_BodyParseAuto_JSON` | `Content-Type: application/json` body becomes a structured `msg.payload` |
| `TestHttpIn_BodyParseAuto_FormURLEncoded` | form body becomes `map[string]string` |
| `TestHttpIn_BodyParseAuto_Multipart` | text + file fields land in expected shape; large file → 413 |
| `TestHttpIn_BodyParseJSON_BadJSON` | invalid JSON returns 400; emits catchable error |
| `TestHttpIn_PathParams` | `/devices/:id` populates `msg.req.params.id` |
| `TestHttpIn_QueryRepeatedKey` | `?a=1&a=2` becomes `query.a == ["1","2"]` |
| `TestHttpIn_RouteConflict` | two http-in with same method+path: both red, neither serves |
| `TestHttpIn_CORSPreflight` | OPTIONS preflight is answered by the server, no flow message emitted |
| `TestHttpIn_DefaultResponseTimeout` | flow without http-response: client receives 504 after `responseTimeout` |
| `TestHttpResponse_Status_Headers_Body` | status/headers/body all flow through |
| `TestHttpResponse_MsgOverridesConfig` | `msg.statusCode` / `msg.headers` win over config |
| `TestHttpResponse_BinaryPayload` | number array → raw bytes, default Content-Type set |
| `TestHttpResponse_NoHandle` | missing `msg.res` → catchable error, no panic |
| `TestHttpResponse_AlreadySent` | second `http-response` for same handle → catchable warning |
| `TestHttpResponse_ClientDisconnect` | client closes connection mid-write → catchable error |
| `TestHttpRequest_BasicGet` | URL config, headers, JSON response → parsed `msg.payload` |
| `TestHttpRequest_MustacheURL` | `{{payload.id}}` resolves from `msg` |
| `TestHttpRequest_MsgOverrides` | `msg.url` / `msg.method` / `msg.headers` win over config |
| `TestHttpRequest_Timeout` | server hangs > timeout → catchable error |
| `TestHttpRequest_NetworkError` | unreachable host → catchable error |
| `TestHttpRequest_NonCanonicalStatus_Passthrough` | 404 emits message with `statusCode=404`, downstream Switch sees it |
| `TestHttpRequest_NonCanonicalStatus_AsError` | `errorMode=error` + 500 → no message, catchable error |
| `TestHttpRequest_RedirectChain` | follows redirects, populates `redirectList` and `responseUrl` |
| `TestHttpRequest_TLSInsecure` | with self-signed server: with insecure flag passes, without flag errors |
| `TestHttpRequest_Auth_Basic` / `_Bearer` | correct `Authorization` header sent |
| `TestHttpRequest_BodyEncoding_Form` | `map` → `application/x-www-form-urlencoded` body |
| `TestEndToEnd_InToResponse` | http-in → function → http-response: caller sees the function's output |
| `TestEndToEnd_Proxy` | http-in → http-request → http-response: payload of the upstream response is forwarded |
| `TestEngineRedeploy_OldHandlesClosed` | requests in flight at redeploy receive 503; new requests hit the new mux |
| `TestEngineStop_DrainsResponseRegistry` | Stop cancels every open slot |
| `TestFlowEndpointMux_NoAuth` | `/endpoint/...` is reachable without session cookie |
| `TestFlowEndpointMux_NoCSRF` | POST to `/endpoint/...` without `X-CSRF-Token` succeeds |
| `TestApiAndEndpointDoNotCollide` | `/api/v1/...` keeps its auth + CSRF; route prefixes do not bleed |

## Dependencies

- **`Catch Node` issue (`CATCH_NODE.md`)** — strongly recommended to
  ship before HTTP, so async errors from `http-request` and
  registration errors from `http-in` are catchable. Not a hard
  blocker (the nodes still log via `publishNodeError`), but the
  user-facing story is much better with Catch in place
- **`PARSER_JSON_NODE.md`** — many real flows will pair `http-in` with
  the JSON parser. Not a code dependency, but mention in the docs
- **`Template Node` engine** — reused for `{{mustache}}` URL
  rendering in `http-request`
- **No** new external Go dependencies. `net/http` and the already
  vendored `chi` cover everything

## Out of Scope / Not in Scope

Deliberately deferred to keep the v1 lean:

- **HTTP/2 and HTTP/3 server features** beyond what `net/http`
  provides out of the box (e.g., explicit Server Push, ALPN tuning).
  ALPN with HTTP/2 is enabled because Go enables it by default; no
  UI for it
- **Server-Sent Events / chunked streaming response** from the
  flow side. `http-response` writes the body in one shot. Streaming
  is a separate node type (e.g., `http-stream-response`)
- **WebSocket inside flow endpoints**. We already have an editor
  WebSocket; flow-side WS is a separate issue
- **OAuth2 client / OIDC flows** as a config node. v1 supports
  Basic + Bearer inline. OAuth is a candidate for a shared `http-auth`
  config node later
- **mTLS** (client certificate authentication) for `http-request`.
  Needs a credential management story which we don't yet have
- **Custom CA bundle** per request. v1 honours system trust anchors;
  insecure-skip is the only escape hatch
- **Rate limiting / DoS protection** on `/endpoint/*`. Operators are
  expected to put a reverse proxy in front for production exposure
- **Per-route auth on `http-in`** (e.g., a built-in API key check).
  The user implements it as a Function node — keeps node concerns
  small. A reusable Auth node is a follow-up
- **File downloads as streaming responses** (`http-response` reading
  from a file path). Caller can build it via Function + Buffer node
- **Cookie jar** for `http-request`. Each request is independent. A
  follow-up `http-session` config node could carry cookies between
  requests
- **Compression negotiation** (gzip/br). `net/http` handles
  `Accept-Encoding: gzip` transparently on the request side; on the
  response side compression is left to the operator's reverse proxy
