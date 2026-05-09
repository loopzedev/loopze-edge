# Certificate store

LOOPZE keeps a central, file-backed certificate catalogue that every
network node can reference by ID instead of embedding PEM inline. The
store is part of the same encrypted file that holds node credentials
(`data/credentials.json`, AES-256-GCM, master key in `data/loopze.key`),
so there is one secret to back up and one master key to rotate.

This page is the operator-facing reference: what you can put in the
store, how to keep it healthy, and how to rotate certificates without
editing flows by hand.

## When to use the store

| Scenario | Recommendation |
|---|---|
| One flow, throwaway dev cert | **Inline PEM** in the node's `tls` block. |
| Many flows hitting the same broker / API | **Stored reference** — one entry, many consumers. |
| Cert is rotated by an external tool (cert-manager, Let's Encrypt, K8s secret), shared across configs | **Stored entry, `Source: file`** — one named entry to manage; many configs reference it. |
| Cert is rotated externally, only one config uses it | **`*File` path directly in the `tls` block** — no store entry needed; same hot-reload semantics. |
| Cert lives only inside LOOPZE | **Inline source** — PEM is encrypted at rest. |

The store and inline mode coexist; you can pick differently per node.

> **Both file-path approaches re-read on every connection init.** A
> stored `Source: file` entry and a direct `caBundleFile` /
> `clientCertFile` / `clientKeyFile` on the `tls` block both work the
> same at runtime. The difference is cataloguing — stored entries get
> a name, fingerprint and expiry in the cert manager UI, plus they
> can be referenced by ID from many configs at once.

## Entry shape

Each entry has:

- **ID** — a slug matching `^[a-z0-9][a-z0-9_-]{0,63}$`. Immutable
  after creation; pick a stable name (`internal-root-ca`,
  `edge-client-2026`).
- **Type** — one of `ca-bundle`, `client-pair`, `server-pair`. Nodes
  type-check on resolution: a `tls.caBundleRef` pointing at a
  `client-pair` is a hard deploy error.
- **Source** — `inline` or `file`.
- **Material** — for `inline`, PEM strings; for `file`, absolute
  paths on disk.
- **Notes** — free-form, surfaces in the UI list.

Fingerprint, subject, issuer and `notAfter` are derived from the
leaf certificate when the entry is saved and shown in the UI.

## Adding a certificate

### Through the UI

1. Open **Certificates** from the header (Editor role or higher).
2. Click **+ New certificate**.
3. Pick the type, choose a source, fill in the material.
4. Click **Validate** to preview the parsed metadata before saving.
5. **Create**.

### Through the API

```bash
# Inline CA bundle
curl -X POST https://loopze.example.com/api/v1/certs \
  -H 'Content-Type: application/json' \
  --cookie loopze_session=… \
  -d '{
        "id":      "internal-root-ca",
        "name":    "Internal Root CA",
        "type":    "ca-bundle",
        "source":  "inline",
        "certPem": "-----BEGIN CERTIFICATE-----\n…\n-----END CERTIFICATE-----"
      }'

# File-sourced client pair
curl -X POST https://loopze.example.com/api/v1/certs \
  -H 'Content-Type: application/json' \
  --cookie loopze_session=… \
  -d '{
        "id":       "edge-client-2026",
        "name":     "Edge gateway client",
        "type":     "client-pair",
        "source":   "file",
        "certPath": "/etc/loopze/certs/edge.crt",
        "keyPath":  "/etc/loopze/certs/edge.key"
      }'
```

The full route table:

| Method | Path                     | Role     |
|--------|--------------------------|----------|
| GET    | `/api/v1/certs`          | Viewer+ |
| GET    | `/api/v1/certs/{id}`     | Viewer+ |
| POST   | `/api/v1/certs`          | Editor+ |
| PUT    | `/api/v1/certs/{id}`     | Editor+ |
| DELETE | `/api/v1/certs/{id}`     | Editor+ (`409` when referenced) |
| POST   | `/api/v1/certs/validate` | Editor+ (probe only — does not persist) |

Responses for `GET` never include PEM material or private keys —
only file paths and parsed metadata. If you need the original PEM
back, you have to re-upload it.

## Rotating certificates

### File source

The cleanest path. The store only persists the file path; the
contents are read fresh on every `BuildTLSConfig` call (which
happens at every node deploy / connection init). External tools
that swap the file in place — cert-manager, Let's Encrypt, mounted
Kubernetes secrets — pick up automatically:

```bash
# Drop the new cert in place atomically; long-lived TLS connections
# stay open until they redial, then use the new material.
mv /etc/loopze/certs/edge.crt.new /etc/loopze/certs/edge.crt
mv /etc/loopze/certs/edge.key.new /etc/loopze/certs/edge.key
```

No restart, no API call, no flow edit.

!!! warning "Long-lived connections survive a rotation"
    The cert file is re-read at connection init, not on every
    write. A connection that was opened before the rotation
    keeps the old TLS material until it closes (or the flow is
    re-deployed). For aggressive rotation, schedule a periodic
    redeploy or close the connection from the peer side.

### Inline source

Inline material lives encrypted inside `credentials.json`. Rotate by
issuing a `PUT` against the entry with the new PEM, or use the UI's
**Edit** button. Connections established before the update keep the
old cert until they redial.

### Tooling examples

=== "cert-manager (Kubernetes)"

    Mount the certificate as a `Secret` and point the LOOPZE entry
    at the projected paths. cert-manager rotates the `Secret`
    contents in place.

    ```yaml
    apiVersion: v1
    kind: Pod
    spec:
      containers:
      - name: loopze
        volumeMounts:
        - name: edge-cert
          mountPath: /etc/loopze/certs
          readOnly: true
      volumes:
      - name: edge-cert
        secret:
          secretName: edge-client-tls
          items:
          - key: tls.crt
            path: edge.crt
          - key: tls.key
            path: edge.key
    ```

=== "Let's Encrypt (host)"

    Point the entry at the `live` symlinks, not at the rotating
    `archive` files:

    ```
    certPath: /etc/letsencrypt/live/example.com/fullchain.pem
    keyPath:  /etc/letsencrypt/live/example.com/privkey.pem
    ```

    Certbot keeps the symlinks pointing at the latest version.

=== "Plain disk"

    Use any deployment tool to swap the files atomically (e.g. write
    a `.new`, `chmod 0600`, then `mv`). LOOPZE does not enforce a
    base directory — the only requirement is that the path is
    absolute.

## Deletion safety net

`DELETE /api/v1/certs/{id}` is rejected with `409 Conflict` when the
saved workspace still references the cert. The response body lists
each reference so you can untangle:

```json
{
  "error":   "Conflict",
  "message": "cert is still referenced by deployed nodes",
  "references": [
    {
      "flowId":   "ingest-line-1",
      "nodeId":   "tcp-out-3",
      "nodeType": "tcp-out",
      "field":    "tls.caBundleRef"
    }
  ]
}
```

The UI surfaces the same list before letting you confirm.

File contents on disk are never touched by `DELETE` — only the store
entry is removed.

## OPC UA migration on upgrade

Existing `opcua-server` config nodes that used `clientCertFile` /
`clientKeyFile` are auto-migrated on first boot after upgrade:

- A new `client-pair` entry is created with deterministic ID
  `opcua-<configNodeID>` and `Source: file`, pointing at the same
  paths.
- The legacy properties are stripped from the config so the
  deprecation WARN does not fire on every subsequent deploy.
- Idempotent: a half-finished migration converges on the next boot.

If you prefer a different naming scheme, run the migration first,
then `PUT` the entries under more meaningful IDs and update the
`certRef` on the affected configs.

## Operational notes

- **One key, one file.** `data/credentials.json` and `data/loopze.key`
  carry the entire store plus node credentials. Back up both, but
  store them separately — the file without the key is useless and
  the key without the file lets nothing through.
- **File permissions.** `0600` on both. The runtime enforces this on
  write; check before backups too.
- **Logging hygiene.** PEM material and private keys are never
  logged. File paths are; if a path is sensitive in your environment
  (rare), avoid putting it on the filesystem.
- **Expired certs are a WARN, not a fail.** Test setups
  legitimately use expired certs. If you want a hard refusal, add
  monitoring on `notAfter` from the UI's listing endpoint.

## Out of scope

The store deliberately does not:

- generate certificates (no "Create CA" button — bring your own PKI);
- speak OCSP / CRL;
- rotate the master key on a schedule;
- integrate with the OS keychain.

These are tracked as separate follow-ups.
