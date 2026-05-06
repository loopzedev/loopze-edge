# Changelog

All notable changes to LOOPZE are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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
