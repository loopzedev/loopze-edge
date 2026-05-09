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

The block supports two source modes per slot (CA bundle and client
pair are independent slots — you can mix them freely):

```json
{
  "tls": {
    "enabled": true,
    "serverName": "device.example.com",

    // Inline mode: PEM embedded in the flow JSON.
    "caBundle":   "-----BEGIN CERTIFICATE-----\n…",
    "clientCert": "-----BEGIN CERTIFICATE-----\n…",
    "clientKey":  "-----BEGIN PRIVATE KEY-----\n…",

    // Reference mode: resolves against the central cert store.
    "caBundleRef":   "ca-internal-root",
    "clientPairRef": "device-2026",

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
| `clientKey`          | —       | Inline PEM-encoded private key.                                |
| `caBundleRef`        | —       | ID of a `ca-bundle` entry in the [cert store](../operations/cert-store.md). |
| `clientPairRef`      | —       | ID of a `client-pair` entry in the cert store. |
| `insecureSkipVerify` | `false` | Disable certificate verification. Loud WARN log on every deploy. |

**Inline and reference are mutually exclusive per slot:** setting both
`caBundle` and `caBundleRef` (or `clientCert`/`clientKey` together with
`clientPairRef`) produces a hard deploy error. Mixing across slots is
fine — e.g. a stored CA bundle paired with an inline client cert.

## Defaults

- **Minimum protocol version: TLS 1.2.** No way to downgrade — older
  versions are deprecated and have known weaknesses.
- **System trust roots** are honoured when `caBundle` is empty. On
  Linux that means `/etc/ssl/certs`; on macOS the system keychain;
  on Windows the certificate store.

## Stored references vs. inline PEM

Inline PEM is fine for one-off flows or development. For production
the [central cert store](../operations/cert-store.md) is preferred
because:

- One stored entry can back many flows. Rotating it updates every
  consumer at once, no per-flow editing.
- File-source entries point at operator-managed paths
  (cert-manager, Let's Encrypt, Kubernetes mounted secrets) and pick
  up new contents on the next connection init — no redeploy needed.
- The cert manager UI surfaces fingerprint, subject and expiry, so
  drift is visible.

Both modes coexist; pick whichever fits the flow.

```yaml
# Reference mode — cert lives in the central store, rotated by ops.
tls:
  enabled: true
  serverName: secure-device.example.com
  caBundleRef:   internal-root-ca
  clientPairRef: edge-2026
```

## mTLS

Set both `clientCert` and `clientKey` to enable mutual TLS. The
node will present the certificate during the handshake; the peer
must request and accept it. Setting only one of the two is rejected
at deploy time.

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

## Legacy fields (deprecated)

A handful of node types accepted simpler TLS knobs before the shared
block existed. They keep working for two minor releases with a WARN
log on every deploy, then are removed:

- `http-request`: `tlsInsecure` (boolean) → migrate to
  `tls.enabled` + `tls.insecureSkipVerify`.
- `mqtt-broker`: `useTLS` (boolean) → migrate to `tls.enabled`.
- `opcua-server`: `clientCertFile` / `clientKeyFile` (paths) → migrate
  to `certRef` against a `client-pair` entry in the cert store.
  Existing flows are auto-migrated on first boot after upgrade
  (deterministic ID `opcua-<configNodeID>`, `Source: file`).

## Out of scope (v1)

- **Server-side TLS** for `tcp-in server`.
- **DTLS** (UDP TLS).
- **OS-keychain integration** for client certificates.
- **OCSP stapling / CRL fetching.**
- **PKI generation** (no "Generate CA" button — bring your own).
