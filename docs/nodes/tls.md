# TLS configuration

Outbound network nodes accept a shared `tls` block that wraps the
underlying connection in a TLS 1.2+ handshake. Today this includes:

- TCP: [`tcp-request`](tcp-request.md), [`tcp-in` client mode](tcp-in.md),
  [`tcp-out` client mode](tcp-out.md)
- HTTP: `http-request`
- MQTT: `mqtt-broker` (used by `mqtt-in`, `mqtt-out`, `mqtt-request`)
- OPC UA: `opcua-server` (uses `certRef` for client-pair material; see
  [Operations → Cert store](../operations/cert-store.md))

The schema is identical across the network nodes. `tcp-in server`
does not support TLS in v1 — server-side TLS is typically terminated
at a reverse proxy in production. UDP nodes have no TLS option (DTLS
is out of scope).

## Schema

The block supports **three source modes per slot** (CA bundle and
client pair are independent slots — you can mix them freely):

```json
{
  "tls": {
    "enabled": true,
    "serverName": "device.example.com",

    // Inline mode: PEM embedded in the flow JSON (encrypted at rest
    // when persisted via the cert store, raw in flow JSON otherwise).
    "caBundle":   "-----BEGIN CERTIFICATE-----\n…",
    "clientCert": "-----BEGIN CERTIFICATE-----\n…",
    "clientKey":  "-----BEGIN PRIVATE KEY-----\n…",

    // Reference mode: resolves against the central cert store.
    "caBundleRef":   "ca-internal-root",
    "clientPairRef": "device-2026",

    // File mode: absolute paths read fresh on every connection init
    // (cert-manager, Let's Encrypt, mounted Kubernetes secrets all
    // rotate transparently — no redeploy needed).
    "caBundleFile":   "/etc/loopze/certs/ca.pem",
    "clientCertFile": "/etc/loopze/certs/client.pem",
    "clientKeyFile":  "/etc/loopze/certs/client.key",

    "insecureSkipVerify": false
  }
}
```

| Field                | Default | Description                                                    |
|----------------------|---------|----------------------------------------------------------------|
| `enabled`            | `false` | Master switch. Without it (or set to `false`), the node speaks plain TCP and the rest of the block is ignored. |
| `serverName`         | —       | SNI / hostname verification target. Falls back to the dial host when empty. |
| `caBundle`           | —       | Inline PEM-encoded CA certificates. Empty = use the system trust roots. |
| `clientCert`         | —       | Inline PEM-encoded client certificate. Pair with `clientKey` for mTLS. |
| `clientKey`          | —       | Inline PEM-encoded private key. PKCS#1 and PKCS#8 are both accepted. |
| `caBundleRef`        | —       | ID of a `ca-bundle` entry in the [cert store](../operations/cert-store.md). |
| `clientPairRef`      | —       | ID of a `client-pair` entry in the cert store. |
| `caBundleFile`       | —       | Absolute path to a CA bundle PEM file. Read on every connection init. |
| `clientCertFile`     | —       | Absolute path to a client cert PEM file. Pair with `clientKeyFile` for mTLS. |
| `clientKeyFile`      | —       | Absolute path to a client private key PEM file. |
| `insecureSkipVerify` | `false` | Disable certificate verification. Loud WARN log on every deploy. |

**At most one source per slot** — setting two of `caBundle`,
`caBundleRef`, `caBundleFile` (or two of the equivalent client-pair
trio) produces a hard deploy error. Mixing across slots is fine — e.g.
a stored CA bundle paired with an inline client cert, or a
file-sourced CA paired with no client auth.

## Defaults

- **Minimum protocol version: TLS 1.2.** No way to downgrade — older
  versions are deprecated and have known weaknesses.
- **System trust roots** are honoured when `caBundle` is empty. On
  Linux that means `/etc/ssl/certs`; on macOS the system keychain;
  on Windows the certificate store.

## Picking a source mode

| Scenario | Recommended mode |
|---|---|
| One-off dev flow against a self-signed cert | Inline PEM |
| Many flows hitting the same broker / API | Stored cert (`*Ref`) |
| Cert is rotated by an external tool (cert-manager, Let's Encrypt, K8s secrets) | Stored cert with `Source: file` **OR** direct `*File` paths in the block |
| Cert lives only inside LOOPZE | Stored cert with `Source: inline` (encrypted at rest) |

The two file-based paths (`caBundleFile`/`clientCertFile`/`clientKeyFile`
on the node config vs. a `Source: file` cert-store entry referenced by
ID) behave the same at runtime — the file is re-read on every
connection init, so external rotation works in both. Stored entries
add a layer of cataloguing (fingerprint, expiry, name) that's useful
when many flows share the same material; direct paths are simpler when
each config has its own files.

```yaml
# Reference mode — cert lives in the central store, rotated by ops.
tls:
  enabled: true
  serverName: secure-device.example.com
  caBundleRef:   internal-root-ca
  clientPairRef: edge-2026

# File mode — paths read fresh on every connection init.
tls:
  enabled: true
  serverName: secure-device.example.com
  caBundleFile:   /etc/loopze/certs/ca.pem
  clientCertFile: /etc/loopze/certs/client.pem
  clientKeyFile:  /etc/loopze/certs/client.key
```

## mTLS

Set both `clientCert` and `clientKey` (inline) or both
`clientCertFile` and `clientKeyFile` (file) to enable mutual TLS. The
node will present the certificate during the handshake; the peer
must request and accept it. Setting only one of the two halves is
rejected at deploy time.

In reference mode, a single `clientPairRef` carries both the cert
and the key — there is no half-pair state to worry about.

```yaml
tls:
  enabled: true
  serverName: secure-device.example.com
  caBundle:   |
    -----BEGIN CERTIFICATE-----
    …
    -----END CERTIFICATE-----
  clientCert: |
    -----BEGIN CERTIFICATE-----
    …
    -----END CERTIFICATE-----
  clientKey: |
    -----BEGIN PRIVATE KEY-----
    …
    -----END PRIVATE KEY-----
```

## `insecureSkipVerify` — warning

Enabling this flag disables hostname and chain verification. It is
useful for development against self-signed certificates but is a
real attacker foothold in production — anyone on the path can
trivially impersonate the peer. The runtime emits a WARN log on
every deploy when the flag is on so the misconfiguration is loud.

Prefer pinning a self-signed cert via `caBundle` over disabling
verification.

## Per-node specifics

Most nodes share the schema above verbatim. Two exceptions:

### `opcua-server`

OPC UA's secure channel needs the **server's** certificate before the
asymmetric `OpenSecureChannel` can be sent. LOOPZE performs an
endpoint discovery automatically when SecurityPolicy ≠ None — no
extra configuration needed. The `tls` block is not used by OPC UA;
instead the cert pair is configured at the OPC UA config level via
`certRef` (preferred) or the legacy `clientCertFile` / `clientKeyFile`
properties on the config root (also supports the same file-rotation
semantics).

OPC UA also requires the cert's SAN URI to match the
`applicationUri`. The default `urn:loopze:client` is what the
included demo cert uses; if you customise `applicationUri` you must
regenerate the cert with the matching URI.

### `mqtt-broker` (config node)

The `tls` block lives on the **broker config node**, not the
`mqtt-in` / `mqtt-out` / `mqtt-request` nodes that reference it.
Operations like `cert-manager`-driven rotation work transparently:
the broker re-reads `*File` paths on every reconnect, so externally
swapped files take effect when the connection cycles.

## Legacy fields (deprecated)

A handful of node types accepted simpler TLS knobs before the shared
block existed. They keep working for two minor releases with a WARN
log on every deploy, then are removed:

- `http-request`: `tlsInsecure` (boolean) → migrate to
  `tls.enabled` + `tls.insecureSkipVerify`.
- `mqtt-broker`: `useTLS` (boolean) → migrate to `tls.enabled`.
- `opcua-server`: `clientCertFile` / `clientKeyFile` (config-level
  paths) → migrate to `certRef` against a `client-pair` entry in the
  cert store. Existing flows are auto-migrated on first boot after
  upgrade (deterministic ID `opcua-<configNodeID>`, `Source: file`).

## Out of scope (v1)

- **Server-side TLS** for `tcp-in server`.
- **DTLS** (UDP TLS).
- **OS-keychain integration** for client certificates.
- **OCSP stapling / CRL fetching.**
- **PKI generation** (no "Generate CA" button — bring your own).
