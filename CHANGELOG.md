# Changelog

All notable changes to LOOPZE are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
