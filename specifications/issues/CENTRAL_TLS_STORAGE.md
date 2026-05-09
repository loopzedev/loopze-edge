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
    ID          string    // user-defined slug (see open question 2)
    Name        string
    Type        string    // "ca-bundle" | "client-pair" | "server-pair"
    CertPEM     string
    KeyPEM      string    // only for *-pair
    Fingerprint string    // SHA-256 over leaf DER
    Subject     string
    Issuer      string
    NotBefore   time.Time
    NotAfter    time.Time
    Notes       string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

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
- Ref resolution in `ParseTLSBlock`
- Migration of all four node types (TCP, HTTP, MQTT, OPC UA)
- Frontend: cert manager view, selector component

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
- [ ] **PR 6 — OPC UA node**: `certRef` property instead of file paths (check library API for DER/key — temp file workaround if needed)
- [ ] **PR 7 — Frontend**: `CertStoreView.vue`, `CertSelector.vue`, mode switch in `TlsConfigSection.vue`

PR 1 → PR 2 sequential. PRs 3–6 can run in parallel. PR 7 depends on PR 3.

## Security

- PEM encrypted at rest via existing AES-256-GCM path
- `GET /api/v1/certs` returns `CertEntrySummary` without `certPem` / `keyPem`
- Never log key material (lint rule on `KeyPEM` in `slog` calls)
- File permissions `0600` on `credentials.json` and `loopze.key` are preserved
- Type mismatch on ref resolution → hard deploy error, no silent fallback

## Tests

- Unit: `CertStore` (PEM parse, validation, roundtrip, fingerprint stability)
- Unit: `ParseTLSBlock` (inline regression, ref mode, conflict cases)
- Integration: TCP/HTTP with `httptest.NewTLSServer` and store refs
- API: CRUD roundtrip, DELETE conflict (409), response hygiene (no PEM leak)

## Open Questions — to decide before PR 1

1. **Storage layout**: same `credentials.json` with version wrapper (recommended) vs. separate `certs.json`?
2. **Cert IDs**: user-defined slugs `^[a-z0-9][a-z0-9_-]{0,63}$` (recommended) vs. UUIDs?
3. **Inline mode**: permanently allowed (recommended) vs. eventual deprecation?
4. **Delete with active reference**: 409 Conflict (recommended) vs. allow orphan refs?
5. **Deprecation cadence** for `useTLS` / `tlsInsecure` booleans — how many releases?
6. **OPC UA**: when both `certRef` and `clientCertFile` are set → conflict error (recommended) vs. file path wins?
7. **Expired certs**: deploy WARN (recommended) vs. deploy fail?

## References

- Forward-looking notes in `DECISIONS.md:85` (Tier 2 — edge identity + cryptographic credentials)
- `PLANNING.md:146` (MQTT broker with CredentialManager integration)
- TCP TLS spec in `docs/nodes/tls.md`
