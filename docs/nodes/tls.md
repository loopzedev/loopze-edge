# TLS configuration

TCP-based outbound nodes ([`tcp-request`](tcp-request.md),
[`tcp-in` client mode](tcp-in.md), [`tcp-out` client mode](tcp-out.md))
accept an optional `tls` block that wraps the underlying connection
in a TLS 1.2+ handshake. The block is identical across all three
nodes.

`tcp-in server` does **not** support TLS in v1 — server-side TLS is
typically terminated at a reverse proxy in production and adding
in-flow listener support multiplies config surface for a low-frequency
need. Tracked as a follow-up.

UDP nodes have no TLS option — DTLS is out of scope.

## Schema

```json
{
  "tls": {
    "enabled": true,
    "serverName": "device.example.com",
    "caBundle":   "-----BEGIN CERTIFICATE-----\n…",
    "clientCert": "-----BEGIN CERTIFICATE-----\n…",
    "clientKey":  "-----BEGIN PRIVATE KEY-----\n…",
    "insecureSkipVerify": false
  }
}
```

| Field                | Default | Description                                                    |
|----------------------|---------|----------------------------------------------------------------|
| `enabled`            | `false` | Master switch. Without it (or set to `false`), the node speaks plain TCP and the rest of the block is ignored. |
| `serverName`         | —       | SNI / hostname verification target. Falls back to the dial host when empty. |
| `caBundle`           | —       | PEM-encoded CA certificates. Empty = use the system trust roots. |
| `clientCert`         | —       | PEM-encoded client certificate. Pair with `clientKey` for mTLS. |
| `clientKey`          | —       | PEM-encoded private key.                                       |
| `insecureSkipVerify` | `false` | Disable certificate verification. Loud WARN log on every deploy. |

## Defaults

- **Minimum protocol version: TLS 1.2.** No way to downgrade — older
  versions are deprecated and have known weaknesses.
- **System trust roots** are honoured when `caBundle` is empty. On
  Linux that means `/etc/ssl/certs`; on macOS the system keychain;
  on Windows the certificate store.

## mTLS

Set both `clientCert` and `clientKey` to enable mutual TLS. The
node will present the certificate during the handshake; the peer
must request and accept it. Setting only one of the two is rejected
at deploy time.

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

## Out of scope (v1)

- **Server-side TLS** for `tcp-in server`.
- **DTLS** (UDP TLS).
- **OS-keychain integration** for client certificates.
- **OCSP stapling / CRL fetching.**
