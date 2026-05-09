# Plan: Central TLS Certificate Storage

Implementation plan for [`CENTRAL_TLS_STORAGE.md`](./CENTRAL_TLS_STORAGE.md). Goal: a shared, reference-able certificate store usable by every connection node, supporting both `inline` (encrypted PEM at rest) and `file` (operator-managed path) sources.

## Locked Decisions

The spec's nine open questions are resolved as follows. Anything below this line is the contract for the implementation:

1. **Storage layout** — single `data/credentials.json` with a v2 envelope `{version, credentials, certs}`. One key file (`loopze.key`), one backup target.
2. **Cert IDs** — user-defined slugs validated against `^[a-z0-9][a-z0-9_-]{0,63}$`. Duplicates → `409 Conflict`.
3. **Inline mode** — permanently supported. Inline and file source coexist as first-class options on the same `CertEntry` type.
4. **Delete with active reference** — `409 Conflict` listing referencing nodes. No orphan refs.
5. **Deprecation cadence** — legacy `useTLS` / `tlsInsecure` booleans keep working for **two minor releases**, with a `WARN` log on every deploy that uses them. Removed in the third release.
6. **OPC UA conflict** — when both `certRef` and `clientCertFile` are set on the same node → hard error at deploy time. Consistent with the inline-vs-ref rule in `ParseTLSBlock`.
7. **Expired certs** — `WARN` at deploy time (`notAfter < now`), no fail. Test setups frequently use expired certs intentionally.
8. **File-source caching** — re-read the file from disk on every `BuildTLSConfig` call. No `fsnotify` watcher in MVP. Connection nodes re-init on deploy; long-lived connections that survive cert rotation are an explicit follow-up issue.
9. **OPC UA auto-migration** — on first start after upgrade, existing `opcua-server` config nodes with `clientCertFile` / `clientKeyFile` are auto-migrated to a `Source: file` cert entry with deterministic ID `opcua-<configNodeID>`. Idempotent: if the ID already exists, skip. Original properties are kept readable for one release, then removed.

## Order

Backend first, smallest verifiable slice first. Each step compiles and is testable in isolation.

```
[1] credentials.CertEntry + validation (no I/O)
   ↓
[2] credentials.json v2 envelope (read+write, backwards compatible)
   ↓
[3] credentials.CertStore (in-memory CRUD + Load/Save)
   ↓
[4] CertStore.BuildTLSConfig (the bridge to tls.Config)
   ↓
[5] ParseTLSBlock refactor + TCP node migration  ← first end-to-end consumer
   ↓
[6] REST API + workspace reference scan         ← unblocks frontend
   ↓
[7] HTTP node migration   ┐
[8] MQTT node migration    ├─ parallelisable after [5]+[6]
[9] OPC UA migration + auto-migration on boot ┘
   ↓
[10] Frontend: cert manager view + selector + TLS section mode switch
   ↓
[11] Docs (docs/nodes/tls.md update + new docs/operations/cert-store.md)
```

After each step: `go test ./...`, then start the server and exercise the smoke criterion noted in the step.

---

## Step 1 — `credentials.CertEntry` + Validation

**Goal:** Pure-data type and validators, zero I/O. Lets us unit-test the rules first.

**New files:**
- `internal/credentials/cert_entry.go` — `CertEntry` struct exactly as in the spec, plus:
  - `const SourceInline = "inline"`, `SourceFile = "file"`
  - `const TypeCABundle = "ca-bundle"`, `TypeClientPair = "client-pair"`, `TypeServerPair = "server-pair"`
  - `var slugRegexp = regexp.MustCompile(\`^[a-z0-9][a-z0-9_-]{0,63}$\`)`
  - `func (e *CertEntry) Validate() error` — checks ID slug, type enum, source enum, mutual exclusion of PEM vs. path fields per source, key required for `*-pair`.
  - `func (e *CertEntry) parseAndPopulate() error` — runs `pem.Decode` + `x509.ParseCertificate` on the leaf cert, fills `Fingerprint` / `Subject` / `Issuer` / `NotBefore` / `NotAfter`. For client pairs additionally calls `tls.X509KeyPair` to detect mismatched cert/key. For file sources, reads the files first.
- `internal/credentials/cert_entry_test.go` — table-driven tests:
  - valid inline ca-bundle
  - valid inline client-pair
  - valid file client-pair (uses `t.TempDir()` + `os.WriteFile`)
  - rejects mixed source (`CertPEM` + `CertPath` set together)
  - rejects bad slug (`Foo`, `with space`, `-leading-dash`, 65-char input)
  - rejects mismatched cert/key pair
  - rejects unknown type / source
  - file source: rejects relative path, rejects non-existent file at validate time

**Smoke:** `go test ./internal/credentials/...` green.

---

## Step 2 — v2 Envelope in `credentials.json`

**Goal:** Read/write the new top-level shape, transparently upgrade old files. No cert logic yet, just the envelope.

**Modified files:**
- `internal/credentials/credentials.go` — add a thin wrapper type for envelope encoding:
  ```go
  type credentialFileV2 struct {
      Version     int                        `json:"version"`
      Credentials map[string]json.RawMessage `json:"credentials"` // unchanged shape
      Certs       map[string]CertEntry       `json:"certs"`
  }
  ```
  And helper functions `encodeEnvelope(creds, certs) ([]byte, error)` / `decodeEnvelope([]byte) (*credentialFileV2, error)` that:
  - On decode: peek at the JSON. If top-level has `"version"` field → v2 path. Otherwise treat the whole document as legacy `credentials` map (v1) and wrap it. Empty input (first run) → fresh v2 with empty maps.
  - On encode: always v2.

**New tests:** `internal/credentials/envelope_test.go`
- v1 file (legacy bare credentials map) decodes with empty `Certs`
- v2 roundtrip preserves both maps
- empty input → empty v2 envelope, no error

**Smoke:** start the server with an existing `credentials.json` from a previous version → boots cleanly, file is unchanged on disk until something writes it; first write upgrades to v2 envelope.

---

## Step 3 — `credentials.CertStore`

**Goal:** In-process CRUD on cert entries, persisted via the v2 envelope. Concurrency-safe, validates on write, returns copies on read.

**New files:**
- `internal/credentials/cert_store.go`:
  ```go
  type CertStore struct {
      cm      *CredentialManager
      storage CertStorage  // see below
      mu      sync.RWMutex
      cache   map[string]CertEntry  // values, not pointers
      creds   map[string]json.RawMessage // pass-through for the credentials half of the envelope
  }

  type CertStorage interface {
      LoadCredentials() ([]byte, error)
      SaveCredentials([]byte) error
  }

  func NewCertStore(cm *CredentialManager, storage CertStorage) *CertStore
  func (s *CertStore) Load() error
  func (s *CertStore) Save() error  // re-encrypts whole envelope

  func (s *CertStore) Store(e CertEntry) (CertEntry, error)   // validates + persists
  func (s *CertStore) Update(id string, e CertEntry) (CertEntry, error)
  func (s *CertStore) Get(id string) (CertEntry, bool)
  func (s *CertStore) List() []CertEntrySummary
  func (s *CertStore) Delete(id string) error
  ```
- `internal/credentials/cert_summary.go`:
  ```go
  type CertEntrySummary struct {
      ID, Name, Type, Source string
      CertPath, KeyPath      string  // populated only when Source == "file"
      Fingerprint, Subject, Issuer string
      NotBefore, NotAfter    time.Time
      Notes                  string
      CreatedAt, UpdatedAt   time.Time
  }
  // Summary INTENTIONALLY omits CertPEM / KeyPEM. Even file paths are returned —
  // operators legitimately need to see where an entry points; only key MATERIAL
  // is hidden.
  ```
- `internal/credentials/cert_store_test.go`:
  - Store + Get roundtrip preserves all fields including derived metadata
  - Store rejects duplicate ID with `ErrCertExists`
  - Update preserves `CreatedAt`, refreshes `UpdatedAt`, re-runs `parseAndPopulate`
  - Delete returns `ErrCertNotFound` for missing ID
  - List returns summaries without `CertPEM` / `KeyPEM`
  - Save → Load roundtrip matches in-memory cache exactly
  - Concurrent Store/Get/List under `-race` is clean
  - Loading legacy v1 file populates `Certs` with empty map, doesn't error

**Smoke:** unit tests green; manual: spin up server, no behavioural change yet.

---

## Step 4 — `CertStore.BuildTLSConfig`

**Goal:** The single function every connection node will call. Resolves a cert ref to a `*tls.Config`, transparently across both source modes.

**Added to `internal/credentials/cert_store.go`:**
```go
type TLSRefOptions struct {
    CABundleRef        string  // optional, must reference a "ca-bundle"
    ClientPairRef      string  // optional, must reference a "client-pair"
    ServerName         string
    InsecureSkipVerify bool
    NodeID             string  // for log context
}

func (s *CertStore) BuildTLSConfig(opts TLSRefOptions) (*tls.Config, error)
```

**Behaviour:**
- For `CABundleRef`: look up entry, type-check (`ca-bundle`), resolve PEM:
  - `Source: inline` → use cached `CertPEM`
  - `Source: file` → `os.ReadFile(CertPath)` fresh on every call (decision #8)
  - feed into `x509.NewCertPool().AppendCertsFromPEM`
- For `ClientPairRef`: look up entry, type-check (`client-pair`), resolve cert+key the same way, build `tls.Certificate` via `tls.X509KeyPair`.
- `ServerName` and `InsecureSkipVerify` flow through unchanged.
- All errors carry `NodeID` for log correlation.
- `WARN` log when the resolved leaf's `NotAfter` is in the past (decision #7).

**New tests in `cert_store_test.go`:**
- Build with valid inline CA-bundle ref → resulting `RootCAs` non-nil
- Build with valid file CA-bundle ref → ditto, contents match what's on disk
- File source picks up rotation: write file A, build config, overwrite file with file B, build again → second build uses B's roots
- Type-mismatch ref (`CABundleRef` pointing to a `client-pair`) → error
- Unknown ref ID → error
- Expired cert → config returned, but `WARN` log captured (use a `slog` test handler)

**Smoke:** still no behaviour change for nodes; this just exercises the new API in unit tests.

---

## Step 5 — `ParseTLSBlock` Refactor + TCP Migration

**Goal:** First node type wired through the store. Inline-PEM TCP flows must remain bit-for-bit unchanged (regression testing); ref-based flows newly work.

**Modified files:**
- `internal/nodes/tls_config.go`:
  - New signature: `func ParseTLSBlock(props map[string]any, nodeID string, certs *credentials.CertStore) (*tls.Config, error)`. The `certs` parameter may be `nil` for tests; if `nil` and any `*Ref` field is set, return a clear error.
  - Read two new optional properties: `caBundleRef` (string), `clientPairRef` (string).
  - Conflict rules (return errors with explicit messages):
    - `caBundleRef` set AND any of `caBundle` set → error
    - `clientPairRef` set AND any of `clientCert`/`clientKey` set → error
  - When a `*Ref` is set: defer to `certs.BuildTLSConfig(...)`, then layer in `serverName` / `insecureSkipVerify` from the block.
  - When no refs set: existing inline path runs unchanged.
- `internal/nodes/tcp_in.go`, `tcp_out.go`, `tcp_request.go`: pass the cert store through. Wiring happens via the node factory closure — see the next subsection.

**Wiring the store into node factories:**
- `internal/flow/registry.go` — extend `NodeFactory` / `ConfigNodeFactory` signatures or the surrounding `Deps` struct (whichever already exists; check on implementation) to include `Certs *credentials.CertStore`. Construct in `cmd/loopze` (or `internal/server/server.go`, wherever the `CredentialManager` is instantiated today) and pass through.
- One change per registration call site, all minimal.

**New / updated tests:**
- `internal/nodes/tls_config_test.go` — keep existing inline-mode tests as regression. Add:
  - `TestParseTLSBlock_RefMode_CABundle` (inline cert in store, ref'd from block)
  - `TestParseTLSBlock_RefMode_ClientPair`
  - `TestParseTLSBlock_RefMode_FileSource` (CA file in `t.TempDir()`)
  - `TestParseTLSBlock_InlineAndRef_Conflict_Errors`
  - `TestParseTLSBlock_NilStoreButRefSet_Errors`
- `internal/nodes/tcp_request_test.go` (or whichever already exists) — one integration test using `httptest`-style setup with TLS, both inline-mode (regression) and ref-mode flows.

**Smoke:** existing TCP-with-TLS flow JSON still works unchanged on disk. New flow JSON with `caBundleRef: "test-ca"` resolves a stored cert correctly.

---

## Step 6 — REST API + Workspace Reference Scan

**Goal:** Operators can manage certs via HTTP. Delete is safe (no orphan refs).

**New files:**
- `internal/api/cert_handlers.go`:
  - `GET    /api/v1/certs` → list `CertEntrySummary` (Viewer role)
  - `GET    /api/v1/certs/{id}` → single summary (Viewer)
  - `POST   /api/v1/certs` → accepts full `CertEntry`, validates, stores (Editor)
  - `PUT    /api/v1/certs/{id}` → updates (Editor)
  - `DELETE /api/v1/certs/{id}` → 204 on success, 409 + JSON listing references when in use (Editor)
  - `POST   /api/v1/certs/validate` → parses input without storing, returns derived metadata (Editor) — used by the frontend "test before save" affordance
- `internal/api/cert_handlers_test.go`:
  - happy-path CRUD
  - response hygiene: `GET` never returns `certPem` / `keyPem`
  - DELETE blocked when a workspace flow references the ID (use a stub workspace)
  - validate endpoint returns `notAfter` etc. without persisting
  - role enforcement: viewer can read, editor can mutate

**Modified files:**
- `internal/api/handlers.go` — add `Certs *credentials.CertStore` to the `Deps` struct.
- `internal/api/routes.go` — register the new routes under the existing role middleware.
- A small `internal/flow/workspace_refs.go` (or extend an existing helper) — `func ScanCertReferences(ws *Workspace, certID string) []NodeRef`. Walks all flows + config nodes, looks at the parsed `tls` block on each (TCP, HTTP, MQTT) and at `certRef` on OPC UA. Returns the list used by DELETE's 409.

**Smoke:** `curl -X POST /api/v1/certs` with a sample CA → success; `GET /api/v1/certs` shows summary with correct `notAfter`; `DELETE` while a flow references it → 409 with the offending node listed.

---

## Step 7 — HTTP Node Migration

**Goal:** `http-request` gets a full `tls` block matching TCP's. Old `tlsInsecure` keeps working for two releases (decision #5).

**Modified files:**
- `internal/nodes/http_request.go`:
  - Add `tls` block parsing via `ParseTLSBlock(props["tls"], nodeID, certs)`.
  - Mapping for legacy: if no `tls` block but `tlsInsecure: true` → synthesize `{enabled: true, insecureSkipVerify: true}` and `slog.Warn("http-request: tlsInsecure is deprecated, migrate to the tls block", "nodeID", nodeID)`.
  - Apply resulting `*tls.Config` to `transport.TLSClientConfig`.

**Tests:**
- legacy `tlsInsecure: true` produces a config with `InsecureSkipVerify: true` and emits a deprecation WARN
- new `tls` block with `caBundleRef` resolves correctly against the store
- both blocks set simultaneously → error (already handled by `ParseTLSBlock`)

---

## Step 8 — MQTT Node Migration

**Goal:** `mqtt-broker` config node gets the same `tls` block. Legacy `useTLS` keeps working for two releases.

**Modified files:**
- `internal/nodes/mqtt_broker.go`:
  - Replace the hardcoded empty `tls.Config{MinVersion: TLS12}` with `ParseTLSBlock(props["tls"], nodeID, certs)`.
  - Legacy: if no `tls` block but `useTLS: true` → synthesize `{enabled: true}` (no certs, no skip-verify — same behaviour as today) + WARN.
  - Scheme selection (`mqtts://` vs `mqtt://`) triggered by either legacy `useTLS` or new `tls.enabled`.

**Tests:**
- legacy `useTLS` keeps working with WARN
- new `tls` block with `clientPairRef` enables mTLS to the broker (uses `mochi-mqtt`/`mqtt.test`-style fake broker if one is available; otherwise a config-only assertion is acceptable)

---

## Step 9 — OPC UA Migration + Auto-Migration on Boot

**Goal:** OPC UA gets `certRef`. Existing flows with `clientCertFile` / `clientKeyFile` are auto-migrated to `Source: file` cert entries on first boot after upgrade (decision #9).

**Modified files:**
- `internal/nodes/opcua_server.go`:
  - New property: `certRef` (string, optional). When set: `entry := certs.Get(certRef)`, then pass to gopcua via `opcua.CertificateFile(entry.CertPath)` / `opcua.PrivateKeyFile(entry.KeyPath)`. Since `Source: file` already keeps the path, gopcua's file API works directly — no temp-file workaround needed.
  - For inline source: write to a per-deploy temp file (cleaned up on node disposal). Acceptable because OPC UA is the only node forced into this; other nodes can use either source freely.
  - Conflict rule: `certRef` set together with any of `clientCertFile`/`clientKeyFile` → error (decision #6).
  - Legacy: if no `certRef` but `clientCertFile`/`clientKeyFile` present → keep working with WARN for two releases.

**New file:**
- `internal/credentials/opcua_migration.go`:
  ```go
  func MigrateOPCUAConfigs(ws *Workspace, certs *CertStore) error
  ```
  - Walk all `opcua-server` config nodes.
  - For each that has `clientCertFile` / `clientKeyFile` AND no `certRef`: build a `CertEntry` with deterministic ID `opcua-<configNodeID>`, `Source: file`, `Type: client-pair`, `Name: "OPC UA: <node name>"`.
  - If `certs.Store(entry)` returns `ErrCertExists` → idempotent, log debug and skip.
  - Set `certRef` on the config node, persist workspace + certs.
  - Called once during server boot, after both stores are loaded.

**Tests:**
- `opcua_migration_test.go`:
  - migrates a single config node end-to-end
  - idempotent: second invocation on already-migrated workspace is a no-op
  - skips nodes that already have `certRef`
  - skips nodes without file paths
  - leaves the original `clientCertFile` / `clientKeyFile` properties in place (for one-release readback)

---

## Step 10 — Frontend

**Goal:** Operators get a UI; flow editors get a cert picker.

**New files:**
- `frontend/src/views/CertStoreView.vue` — list, create, edit, delete; mode-switch `Inline PEM` / `File path`; "Validate" button hits the `/validate` endpoint and shows derived metadata before save.
- `frontend/src/components/config/CertSelector.vue` — dropdown filtered by `type`, populated from a Pinia store; emits the selected ID.
- `frontend/src/stores/certs.ts` — Pinia store: `list()`, `get(id)`, `create()`, `update()`, `delete()`, plus an in-memory cache invalidated on mutations.

**Modified files:**
- `frontend/src/components/config/TlsConfigSection.vue` — segmented control with three modes:
  - **Disabled** (`enabled: false`)
  - **Inline PEM** (today's UI; CA bundle + optional client cert/key textareas)
  - **Stored cert** (two `CertSelector`s — CA bundle picker, client pair picker)
  Mode switch clears the fields belonging to the inactive mode so the backend never sees a conflict.
- `frontend/src/router/index.ts` — add the `/certs` route, gated to Editor+.
- Sidebar / nav — add a "Certificates" entry under the existing settings section.

**Smoke:** create a CA via UI, build a TCP node referencing it, deploy, observe a TLS connection succeed.

---

## Step 11 — Docs

**Modified:**
- `docs/nodes/tls.md` — add a "Stored certificates" section with both source modes; keep the existing inline-PEM section as primary for casual users.
- `docs/getting-started/concepts.md:52` — extend the `credentials.json` description to mention `certs`.
- `docs/deployment/index.md`, `docs/deployment/docker.md` — note that file-source paths must be reachable from inside the container; recommend mounting cert directories read-only.

**New:**
- `docs/operations/cert-store.md` — operator-focused: how to add/rotate certs, the difference between inline and file sources, examples for cert-manager + Let's Encrypt + plain disk.

---

## Out-of-Scope Tracker (for follow-up issues)

These are intentionally NOT in this plan; reference them in the PR description so they are not lost:

- PKI generation (self-signed CA / leaf cert via UI button)
- OCSP / CRL validation
- `fsnotify`-based file-source caching with hot reload to live connections
- PKCS#12 import
- Scoped multi-tenant cert visibility
- Re-encryption flow when `loopze.key` rotates
- Live cert reload for long-running connections without redeploy
