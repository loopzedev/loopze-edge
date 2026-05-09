# Changelog

All notable changes to LOOPZE are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.8] - 2026-05-09

### Added
- **Central TLS certificate store.** A new shared catalogue of TLS material that every network node (TCP, HTTP, MQTT, OPC UA) can reference by ID instead of embedding PEM inline. Two source modes per entry: `inline` (PEM stored encrypted at rest in `credentials.json`) and `file` (the store keeps the absolute path and re-reads the contents on every connection init, so cert-manager / Let's Encrypt / Kubernetes-mounted-secret rotations take effect without a redeploy). Entries are typed (`ca-bundle` / `client-pair` / `server-pair`); type-mismatched references fail at deploy time. The leaf certificate's fingerprint, subject, issuer and `notAfter` are derived at save time and surfaced in the UI.
- **Three source modes on every `tls` block** — `caBundle` / `caBundleRef` / `caBundleFile` for the CA slot, and the analogous trio for the client pair. At most one source per slot; mixing across slots is allowed. File mode reads from disk on every connection init, providing the same hot-reload semantics as a `Source: file` cert-store entry but without the cataloguing overhead — useful when only a single config consumes the material.
- **REST API for the cert store** — `GET/POST/PUT/DELETE /api/v1/certs` plus `POST /api/v1/certs/validate` for "test-before-save". Viewer reads, Editor mutates. `GET` responses never include PEM material or private keys; only paths and parsed metadata are surfaced. `DELETE` is rejected with `409 Conflict` and a list of referencing nodes when the workspace still depends on the entry. `PUT` treats omitted PEM/path fields as "keep existing material" so an editor can rename or annotate an entry without re-supplying its secrets.
- **Cert manager UI.** New `/certs` view (Editor+) with a sortable list, create/edit modal supporting both source modes, slug validation matching the backend regex, parse-preview button hitting `/validate`, and a delete dialog that surfaces the workspace references blocking a removal. `TlsConfigSection.vue` gains a source-mode toggle so node properties offer either inline PEM textareas or a typed cert selector. The MQTT broker config exposes the same toggle with `Disabled` / `Stored cert` / `File path` modes; OPC UA likewise gains a `Stored cert` / `File path` switch on its certificate-auth panel.
- **Auto-migration for OPC UA configs.** On first boot after upgrade, every `opcua-server` config node with legacy `clientCertFile` / `clientKeyFile` properties is converted into a `Source: file` cert-store entry with deterministic ID `opcua-<configNodeID>`, and the legacy properties are removed so the deprecation WARN does not fire on subsequent deploys. Idempotent.
- **OPC UA endpoint discovery.** When `SecurityPolicy != None`, the runtime now performs a `GetEndpoints` call before opening the secure channel and feeds the server's certificate (plus matching policy / mode) via `opcua.SecurityFromEndpoint`. Without this, gopcua's `OpenSecureChannel` aborts with a confusing "x509 malformed format" error because the remote certificate is unset. User-identity wiring is also fixed: `authMode: certificate` now correctly attaches the cert via `AuthCertificate` / `AuthPrivateKey` (previously `AuthAnonymous` was used by mistake even when a client cert was configured).
- **In-process PEM parsing for OPC UA.** Cert and key material is parsed inside LOOPZE and handed to gopcua via `Certificate(der)` / `PrivateKey(*rsa.PrivateKey)` instead of relying on its file-based loaders, which only accept PKCS#1 RSA keys. PKCS#1, PKCS#8, and EC keys all produce clear errors or Just Work; the previous tempfile dance is gone.
- **Bundled MQTT demo broker** (`demo/mqtt-broker/`) — Mosquitto-2 in Docker Compose with three listeners (`:1883` plain, `:8883` server-TLS, `:8884` mTLS) and a self-signed CA + server cert + client cert checked into the repo so cert-based flows work out of the box. `make demo-mqtt` / `make demo-mqtt-stop` start and stop the broker.
- **Bundled OPC UA demo client cert** (`demo/opcua-server/fixtures/client/`) — RSA-2048 PKCS#1 cert pair with SAN URI `urn:loopze:client` (matching the LOOPZE default `applicationUri`), auto-installed into the demo server's trusted store on every boot. Pair it with a `client-pair` cert-store entry to test OPC UA `certRef` end-to-end.
- **Documentation** — new [Operations → Cert store](https://docs.loopze.dev/operations/cert-store/) page (operator-facing rotation guide with cert-manager / Let's Encrypt / plain-disk recipes); [TLS configuration](https://docs.loopze.dev/nodes/tls/) extended with the three-mode schema, per-node specifics for OPC UA / MQTT, and the legacy-fields deprecation table; [`mqtt-in`](https://docs.loopze.dev/nodes/mqtt-in/) gains a TLS subsection on its broker-config-node reference with examples for both ref and file mode.

### Changed
- **`http-request` `tlsInsecure` is deprecated** in favour of the structured `tls` block. It keeps working for two minor releases with a WARN log on every deploy.
- **`mqtt-broker` `useTLS` is deprecated** in favour of the structured `tls` block. Same two-release deprecation window.
- **`opcua-server` `clientCertFile` / `clientKeyFile` are deprecated** in favour of `certRef` against a `client-pair` entry in the cert store. The auto-migrator handles existing flows; manual rewrites are unnecessary.
- **`credentials.json` schema is now v2** with a top-level envelope `{version, credentials, certs}`. Legacy v1 files (a bare credentials map) are auto-upgraded on first read; the file is rewritten in v2 shape on the next save. Future versions are rejected with a clear error so an older binary never silently drops fields written by a newer one.

### Internal
- New `internal/credentials/cert_entry.go`, `cert_store.go`, `cert_summary.go`, `envelope.go` — `CertEntry` type with `Validate()` / `parseAndPopulate()` / `loadMaterial()`, the `CertStore` itself with concurrency-safe CRUD (mutations roll back on save failure), `BuildTLSConfig` / `LoadMaterial` resolvers re-reading file-source entries fresh per call.
- New `flow.CertStoreProvider` interface; the engine injects the store before `Init` for `NodeInstance` and before `Start` for `ConfigInstance`.
- New `flow.ScanCertReferences(ws, certID)` walks flows + config nodes and returns every `tls.caBundleRef` / `tls.clientPairRef` match — used by the API's 409 reference list.
- `OpcuaTestConnect` signature extended with `*credentials.CertStore` so the `/api/v1/opcua/test-connection` endpoint resolves cert refs the same way the runtime does.
- `internal/nodes/opcua_server.go` — security/auth options moved out of the factory and into `applyCertOptionsLocked`, called per connect with the necessary context for `GetEndpoints`. New `decodePEMCertificate` / `decodePEMRSAKey` helpers handle both PKCS#1 and PKCS#8 keys natively.
- `internal/nodes/tls_config.go` — `boolCount` enforces at-most-one-source semantics across the three input modes per slot.

## [0.0.7] - 2026-05-08

### Added
- **`mqtt-request` node** — synchronous MQTT v5 request/response in a single node. For each input message the node generates a unique response topic (`<prefix>/<uuid>`) plus 16 random bytes of correlation data, opens a one-shot subscription, publishes the request with v5 `Response Topic` + `Correlation Data` properties, and waits for the matching reply or for the timeout. Two timeout modes: `error` (catchable error) and `passthrough` (msg with `msg.timedOut=true`). Multiple inflight requests are tracked in parallel via an internal map keyed by hex-encoded correlation data; on broker disconnect or `Stop()` every pending context is drained cleanly. Default v5 publish properties (user properties, content type, message expiry, payload format) can be configured and per-message-overridden the same way as on `mqtt-out`.
- **`mqtt-out` `target` selector** — new `target` field with values `topic` (default, current behaviour) and `responseTopic`. In response-topic mode the node publishes to `msg.responseTopic` (ignoring the configured topic and `msg.topic`) and forwards `msg.correlationData` as the v5 Correlation Data property. Pairs with a remote `mqtt-request` to close the round-trip with a single line of glue (`[mqtt-in] → [Function] → [mqtt-out target=responseTopic]`).
- **Per-subscriber connection-down callbacks on the MQTT broker manager.** New `RegisterConnectionDownFunc(subscriberID, fn)` / `UnregisterConnectionDownFunc(subscriberID)` API on `MqttBroker`. Used by `mqtt-request` to fail every inflight context the moment the broker drops, instead of waiting for each individual timer.
- **Documentation.** Three new node reference pages — [MQTT Subscribe](https://docs.loopze.dev/nodes/mqtt-in/), [MQTT Publish](https://docs.loopze.dev/nodes/mqtt-out/) (with the new target selector), and [MQTT Request](https://docs.loopze.dev/nodes/mqtt-request/). MkDocs nav and the nodes overview index updated.
- **Frontend.** New `MqttRequestConfig.vue` properties panel; `MqttNodeConfig.vue` extended with the publish-target toggle and a hint block when "Response to responseTopic" is selected; `mqtt-request` palette token + canvas rendering with MQTT brand icon; `BaseNode.vue` default label.

### Changed
- `mqtt-out` `correlationData` accepts three wire formats. `[]byte` is passed through as before; **base64-encoded strings** are decoded back to the original bytes (Go's default `[]byte → JSON` shape — surfaces when a msg has travelled through a function node or NATS routing); `[]int` / `[]any` of numbers are byte-packed (the JSON shape of an `[]int` payload from `mqtt-in` buffer mode). The previous string path interpreted the base64 ASCII as literal bytes, breaking request/response correlation through any JSON-serializing intermediate.

### Fixed
- **`mqtt-request` timed out even when the responder published the reply correctly.** The internal response-topic subscription was opened with `NoLocal=true`. Per MQTT v5 §3.8.3.1 the broker filters messages from a connection with the same Client ID — and request and responder share the broker connection in any single LOOPZE instance, so the response was being filtered out. Fixed by setting `NoLocal=false`; the random response topic already prevents echo loops.

## [0.0.6] - 2026-05-08

### Added
- **TCP nodes** — `tcp-in`, `tcp-out`, and `tcp-request`, enabling flows to act as TCP servers, TCP clients, or to perform synchronous TCP round-trips:
  - `tcp-in` (server / client modes) listens on a port or dials a remote, applies one of four [framing strategies](https://docs.loopze.dev/nodes/framing/) (`stream` / `delimiter` / `length-prefix` / `fixed-length`), and emits one message per frame. Server mode registers each accepted connection with the engine's `SessionRegistry` so paired `tcp-out reply` nodes can write back. Client mode reconnects with exponential backoff + ±20% jitter. CIDR allowlist for incoming peers; `emitCloseEvent` opt-in to surface peer disconnects on the data path.
  - `tcp-out` writes `msg.payload` to a TCP peer. Three modes: `reply` (looks up `msg.session`), `server-broadcast` (fan-out to every active session of a sibling `tcp-in`), and `client` (dial-and-send, with optional persistent connection + bounded outbound queue + transparent reconnect-and-retry).
  - `tcp-request` is the synchronous primitive: dial → send → read response → close, with five terminator strategies (`time`, `delimiter`, `length`, `length-prefix`, `close`) reusing the framing module. Optional `keepConnection=true` keeps a single TCP conn open across requests, transparently redialing on a stale socket.
- **UDP nodes** — `udp-in` and `udp-out`:
  - `udp-in` binds a UDP port, optionally joins one or more IPv4 multicast groups (with `224.0.0.251%eth0` interface-pinning), and emits one message per datagram. Honours a CIDR allowlist; reports `truncated=true` when datagrams exceed `maxDatagramBytes`.
  - `udp-out` sends `msg.payload` as unicast / broadcast (`SO_BROADCAST`) / IPv4 multicast (configurable TTL, loopback, outbound interface). `reuseSocket=true` keeps one outbound UDP socket open across messages.
- **TLS for outbound TCP.** A shared `tls` config block on `tcp-request`, `tcp-in client`, and `tcp-out client` wraps the connection in a TLS 1.2+ handshake. Supports SNI override, custom CA bundle (PEM), mutual TLS via `clientCert` + `clientKey`, and an explicit `insecureSkipVerify` escape hatch (loud WARN log on every deploy).
- **Engine-owned `SessionRegistry`.** A new `internal/flow.SessionRegistry` mirrors the existing `ResponseRegistry` pattern: TCP sessions are addressable by opaque `*flow.SessionHandle` tokens, survive Link-Out → Link-In hops across flows, and are guarded by per-slot mutex so concurrent fan-out from upstream branches is serialised. The engine drains all sessions on `Stop()`.
- **Shared framing module** (`internal/nodes/framing.go`) — `Framer` interface plus four implementations (`stream`, `delimiter`, `length-prefix`, `fixed-length`), used by both `tcp-in` and `tcp-request`. Includes `ParseDelimiter` for JS-style escape syntax (`\n`, `\r\n`, `\xFF`, …) and `ParseEndianness` for big/little-endian length prefixes.
- **Documentation.** New `docs/nodes/` reference pages for all five new nodes plus shared explainers for [framing](https://docs.loopze.dev/nodes/framing/) and [TLS configuration](https://docs.loopze.dev/nodes/tls/).
- **Frontend.** Property panels (`TcpInConfig.vue`, `TcpOutConfig.vue`, `TcpRequestConfig.vue`, `UdpInConfig.vue`, `UdpOutConfig.vue`) and a shared `TlsConfigSection.vue`. Two new node-palette colour tokens — `tcp` (deep teal `#3da99a`) and `udp` (warm amber `#d6a04b`) — pairing the two transport families as a cool/warm Yin-Yang on the canvas.
- **HTTP nodes** — `http-in`, `http-response`, and `http-request`, enabling flows to act as HTTP endpoints (webhooks, REST APIs) and to call external HTTP services:
  - `http-in` registers a route (method + path) on the flow endpoint mux. Supports path parameters, query strings, and configurable body parsing.
  - `http-response` writes the reply for a matching `http-in` request, with at-most-once semantics enforced by a `ResponseRegistry` (`sync.Once` per handle, deadline sweeper, and `DrainAll` on engine shutdown / redeploy).
  - `http-request` performs outbound HTTP calls with configurable method, headers, body, and timeout.
  - Frontend config panels for all three node types and FlowEditor rendering with method/path/status badges.
- **Flow endpoint mux.** `FlowEndpointMux` in `internal/server` is backed by `atomic.Pointer[chi.Router]`: in-flight handlers continue on the captured pointer while new requests hit the post-swap router. Detects (method, path) conflicts at deploy time and reports both sides via `errorFn`.
- **`/endpoint` mount.** The configured HTTP node root (`/endpoint` by default, configurable via `--http-node-root` / `LOOPZE_HTTP_NODE_ROOT`) is mounted before `/api/v1` with no auth and no CSRF, so flow-defined HTTP endpoints can act as public webhooks.
- **Documentation site.** New `docs/` tree built with MkDocs Material, served from a custom domain via GitHub Pages. Includes getting-started guide, node reference, architecture notes, and deployment guide. Makefile targets (`docs-build`, `docs-serve`, versioned deploy) and a `.github/workflows/docs.yml` workflow.

### Changed
- Engine rebuilds the flow endpoint mux after every `wireAllNodes` call (deploy, partial deploy, node removal); the registry is drained on `Stop`.
- Internal node/feature specifications moved from `docs/issues/` to `specifications/issues/` so the public docs tree only contains user-facing content.

## [0.0.3] - 2026-05-03

### Added
- **Reverse-proxy friendliness.** The runtime is now production-ready behind nginx, Caddy, Traefik & co.:
  - `--base-path` / `LOOPZE_BASE_PATH` mounts the entire app (UI, API, WebSocket) under a configurable URL prefix (e.g. `/loopze`) without rebuilding the frontend. The backend injects a `<base href>` and a `window.__LOOPZE_BASE__` global into `index.html` at request time; the SPA reads them to derive Vue-Router base, API URLs and WebSocket URL.
  - `--trusted-proxies` / `LOOPZE_TRUSTED_PROXIES` — CIDR/IP allowlist whose `Forwarded` (RFC 7239), `X-Forwarded-For`, `X-Real-IP` headers are honoured. Default empty means "trust no upstream", so spoofed headers from the open internet are ignored.
  - `--trusted-origins` / `LOOPZE_TRUSTED_ORIGINS` — WebSocket origin allowlist with `*.example.com` wildcard support. Default falls back to same-origin.
- **CSRF protection on the REST API** via the double-submit-cookie pattern. A random `loopze_csrf` token is issued on every request; mutating methods (POST/PUT/PATCH/DELETE) must echo it back as `X-CSRF-Token`. The SPA does this automatically.
- `/health` endpoint is now reachable both at the root (`/health`) and under the configured base path (`/<basepath>/health`), so probe configuration stays simple.
- Startup log line now includes the configured base path.

### Changed
- `Secure` attribute on session and CSRF cookies is now decided **per request** based on `r.TLS != nil` or `X-Forwarded-Proto: https`. This removes the boot-time choice between secure and insecure cookies — the same binary works on `http://localhost`, in a LAN over plain HTTP, and behind a TLS-terminating proxy without configuration.
- WebSocket `CheckOrigin` is no longer permissive; it enforces same-origin (default) or the configured origin allowlist.
- Frontend asset URLs are now relative (`base: './'` in Vite + relative `<link>`/`<script>` hrefs in `index.html`), so `<base href>` rewriting works correctly under any subpath.
- `flowStore.deploy()` now goes through the central `useApi` request helper (was a raw `fetch` that bypassed the CSRF header).

### Removed
- `--auth-insecure-cookies` flag and `LOOPZE_AUTH_INSECURE_COOKIES` env var. Replaced by per-request scheme detection (see above).

## [0.0.2] - 2026-05-03

### Added
- `--version` / `-v` flag: prints version, commit hash and build time, then exits.
- ASCII-art startup banner showing version and commit, printed before the structured logger is wired up.

### Changed
- Embedded NATS broker logs at debug level (was info), so default-level output stays focused on application events.
- CI workflow (`.github/workflows/ci.yml`) triggers tightened; release archives now disambiguate ARM v6/v7 by name.
- CI runs the Go race detector; `DelayNode` status update locking fixed accordingly.

## [0.0.1] - 2026-05-03

Initial public release.

### Changed
- **License: relicensed from Elastic License 2.0 (ELv2) to AGPL-3.0-or-later.** Strong copyleft + § 13 (SaaS clause) prevents embedding into proprietary products. Copyright transferred to Dennis Bleul personally.
- All Go source-file headers updated to the new license.
- All German documentation, source comments, and UI strings translated to English (~12,500 lines across 46 files) in preparation for public release.

### Added
- `NOTICE` file with copyright notice and source-code URL.
- License + source-code link surfaced in the Settings view to satisfy AGPL § 13 (network users must be able to obtain the source).
- `CONTRIBUTING.md` with PR workflow and an inbound=outbound licensing clause that preserves dual-licensing flexibility.
- `SECURITY.md` with private vulnerability-reporting process and disclosure timeline.
- `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1).
- GitHub Actions workflow (`.github/workflows/ci.yml`) running `go vet` / `go build` / `go test` and a frontend type-check + build on push and pull request.

### Removed
- Accidentally tracked `opcua-smoke` smoke-test binary (7.5 MB ELF) removed from the index.

---

> Earlier development history is preserved in git but is not retroactively
> documented here — this changelog starts with the public release preparation.
