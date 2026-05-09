# Central TLS Certificate Storage for Connection Nodes

## Motivation

TLS handling is currently inconsistent across connection nodes:

| Node | Current state | File |
|---|---|---|
| TCP (in/out/request) | Clean shared `ParseTLSBlock`, PEM embedded in flow JSON | `internal/nodes/tls_config.go:35` |
| HTTP-Request | Only `tlsInsecure` boolean — no mTLS, no CA bundle | `internal/nodes/http_request.go:166` |
| MQTT-Broker | `useTLS` boolean → hardcoded empty `tls.Config{MinVersion: TLS12}` | `internal/nodes/mqtt_broker.go:203` |
| OPC UA-Server | File paths (`clientCertFile`/`clientKeyFile`), not PEM-embedded | `internal/nodes/opcua_server.go:146` |

Four different approaches to the same problem. Cert rotation requires editing PEM in every affected flow. mTLS support for HTTP/MQTT is missing entirely.

The existing `CredentialManager` (`internal/credentials/credentials.go`) already provides AES-256-GCM encryption for credentials. Certificates are conceptually the same problem (secret at rest, referenced by ID) — we can extend this infrastructure instead of building a new subsystem.

## Goal

A central cert storage that connection nodes can use via reference ID instead of inline PEM — inline mode remains opt-in for small setups.

The store supports two source modes per entry:

- **`inline`**: PEM material is pasted/uploaded once and stored encrypted at rest in `credentials.json` (AES-256-GCM).
- **`file`**: PEM material lives on disk at an operator-managed path; the store only persists the path plus parsed metadata. Contents are re-read fresh at every connection init, so external rotation (cert-manager, Let's Encrypt renewals, mounted Kubernetes secrets, OPC UA's existing file-based setup) works without touching the store.

From a node's perspective both modes look identical — the node references a cert by ID and `CertStore.BuildTLSConfig` resolves it transparently.

## Architecture Sketch

- New `CertStore` in `internal/credentials/certs.go`, reusing `CredentialManager` for encrypt/decrypt
- `credentials.json` migrates to v2 schema: `{version, credentials, certs}` (backwards compatible on read)
- `ParseTLSBlock` accepts either `caBundleRef` / `clientPairRef` **or** inline PEM (mutually exclusive → error on conflict)
- Validation on save: parse PEM, `tls.X509KeyPair` for client pairs, derive fingerprint / `NotAfter` / subject
- REST API for CRUD, GET never returns key material
- Frontend gets a cert manager view and selector component for TLS config sections

### Cert Entry Data Model

```go
type CertEntry struct {
    ID     string // user-defined slug (see open question 2)
    Name   string
    Type   string // "ca-bundle" | "client-pair" | "server-pair"
    Source string // "inline" | "file"

    // Source == "inline": PEM is stored encrypted inside credentials.json.
    CertPEM string
    KeyPEM  string // only for *-pair

    // Source == "file": absolute paths on disk; contents are read fresh
    // at every BuildTLSConfig call so external rotation works.
    CertPath string
    KeyPath  string // only for *-pair

    // Derived from the certificate at store time (regardless of source)
    // and refreshed on Update / re-validation.
    Fingerprint string // SHA-256 over leaf DER
    Subject     string
    Issuer      string
    NotBefore   time.Time
    NotAfter    time.Time

    Notes     string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

**Validation rules:**
- `Source == "inline"`: `CertPEM` required; `KeyPEM` required for `*-pair` types; path fields must be empty.
- `Source == "file"`: `CertPath` required (must be absolute, file must exist and be readable at save time); `KeyPath` required for `*-pair`; PEM fields must be empty.
- For both sources: PEM is parsed at save time to derive `Fingerprint`/`Subject`/`NotAfter`; mismatched cert/key pairs are rejected via `tls.X509KeyPair`.

**Resolution at deploy / connection init:**
- `inline` → decrypt PEM from in-memory cache and feed into `tls.Config`.
- `file` → re-read file from disk on each call; if the file is missing or unparseable, return a hard error to the calling node (deploy fails with a clear message). Optional future improvement: in-memory caching with `fsnotify`-based invalidation; out of scope for MVP.

### `tls` Block Schema

```jsonc
// Inline (today, unchanged)
{ "enabled": true, "caBundle": "-----BEGIN…", "clientCert": "…", "clientKey": "…" }

// By reference (new)
{ "enabled": true, "caBundleRef": "ca-internal-root", "clientPairRef": "device-2026" }
```

## Scope

### In Scope (MVP)
- PEM storage (encrypted at rest) + CRUD API
- File-path source for externally-managed cert files (operator-rotated)
- Ref resolution in `ParseTLSBlock` (transparent across both source modes)
- Migration of all four node types (TCP, HTTP, MQTT, OPC UA) — OPC UA's existing file paths become first-class `Source: file` entries in the store
- Frontend: cert manager view, selector component, source-mode switch on create/edit

### Out of Scope (intentional)
- No PKI generation (no "Generate CA" button)
- No OCSP validation
- No auto-rotate before `notAfter`
- No OS keychain integration
- No PKCS#12 import (PEM only)
- No multi-tenant scoping
- No re-encryption when `loopze.key` changes (separate issue)

## Implementation — Milestones / Sub-PRs

- [ ] **PR 1 — Backend foundation**: `CertStore` + `CertEntry` + schema migration in `credentials.json` (v2 wrapper). Inline mode keeps working unchanged. *Files: `internal/credentials/certs.go` (new), `internal/credentials/credentials.go`*
- [ ] **PR 2 — ParseTLSBlock + TCP**: New signature with optional `*CertStore`, ref resolution, conflict handling. TCP nodes as first consumer. *Files: `internal/nodes/tls_config.go`, `internal/nodes/tcp_{in,out,request}.go`*
- [ ] **PR 3 — REST API**: `/api/v1/certs` CRUD, workspace reference scan on DELETE (409 when reference is in use). *Files: `internal/api/cert_handlers.go` (new), `internal/api/routes.go`*
- [ ] **PR 4 — HTTP node**: Introduce `tls` block, deprecate `tlsInsecure` (mapping + WARN log)
- [ ] **PR 5 — MQTT node**: Introduce `tls` block, deprecate `useTLS`
- [ ] **PR 6 — OPC UA node**: `certRef` property instead of inline file paths. Migration is straightforward: existing `clientCertFile` / `clientKeyFile` flows can be auto-migrated into a `Source: file` cert entry on first load (or operator does it manually). Library can keep using file paths since the store now owns them.
- [ ] **PR 7 — Frontend**: `CertStoreView.vue`, `CertSelector.vue`, mode switch in `TlsConfigSection.vue`

PR 1 → PR 2 sequential. PRs 3–6 can run in parallel. PR 7 depends on PR 3.

## Security

- `inline` source: PEM encrypted at rest via existing AES-256-GCM path.
- `file` source: paths are stored in clear (paths are not secrets); the actual cert files on disk are the operator's responsibility (recommended: `0600` on key files, dedicated cert directory). The store does NOT copy or re-encrypt file contents.
- `GET /api/v1/certs` returns `CertEntrySummary` without `certPem` / `keyPem` — for `file` entries the path IS exposed (operators legitimately need to see where it points).
- Never log key material (lint rule on `KeyPEM` / file contents in `slog` calls). Paths may be logged.
- File permissions `0600` on `credentials.json` and `loopze.key` are preserved.
- Type mismatch on ref resolution → hard deploy error, no silent fallback.
- File-source path constraints: must be absolute. Symlinks are followed (operators may rely on this for rotation via symlink swaps). No allowlist of base directories in MVP — admin-only API surface makes path traversal a non-issue. Re-evaluate if a less-trusted role is added later.

## Tests

- Unit: `CertStore` inline source (PEM parse, validation, encrypt roundtrip, fingerprint stability)
- Unit: `CertStore` file source (path validation, file-not-found at save → reject, file-removed-after-save → BuildTLSConfig errors clearly, external rotation picked up on next call)
- Unit: `CertStore` rejects mixed-source entries (e.g. both `CertPEM` and `CertPath` set)
- Unit: `ParseTLSBlock` (inline regression, ref mode, conflict cases — ref mode test parametrised over both source types)
- Integration: TCP/HTTP with `httptest.NewTLSServer`, exercising both inline and file-sourced refs
- API: CRUD roundtrip per source mode, DELETE conflict (409), response hygiene (no PEM leak; paths exposed by design)

## Open Questions — to decide before PR 1

1. **Storage layout**: same `credentials.json` with version wrapper (recommended) vs. separate `certs.json`?
2. **Cert IDs**: user-defined slugs `^[a-z0-9][a-z0-9_-]{0,63}$` (recommended) vs. UUIDs?
3. **Inline mode**: permanently allowed (recommended) vs. eventual deprecation?
4. **Delete with active reference**: 409 Conflict (recommended) vs. allow orphan refs?
5. **Deprecation cadence** for `useTLS` / `tlsInsecure` booleans — how many releases?
6. **OPC UA**: when both `certRef` and `clientCertFile` are set → conflict error (recommended) vs. file path wins?
7. **Expired certs**: deploy WARN (recommended) vs. deploy fail?
8. **File-source caching**: re-read on every `BuildTLSConfig` call (recommended for MVP — simple, supports any external rotation) vs. cache + `fsnotify`-based invalidation (faster but adds a watcher per entry)?
9. **OPC UA auto-migration**: on first start after upgrade, automatically convert existing `clientCertFile` / `clientKeyFile` properties into store entries with deterministic IDs (recommended) vs. require operator to migrate manually?

## References

- Forward-looking notes in `DECISIONS.md:85` (Tier 2 — edge identity + cryptographic credentials)
- `PLANNING.md:146` (MQTT broker with CredentialManager integration)
- TCP TLS spec in `docs/nodes/tls.md`
