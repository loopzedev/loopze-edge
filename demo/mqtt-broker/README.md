# LOOPZE MQTT Demo Broker

A pre-configured Eclipse Mosquitto broker that exposes three listeners
on different ports — one per TLS permutation a LOOPZE flow might want
to test. Bundled CA + server cert + client cert make the cert-based
flows reproducible without operators having to run their own PKI.

## Quick start

```bash
cd demo/mqtt-broker
docker compose up -d
docker compose logs -f          # follow broker logs
docker compose down             # state in ./data is kept
```

The broker is now reachable on:

| Port | Mode    | Auth on the wire                          |
| ---- | ------- | ----------------------------------------- |
| 1883 | plain   | none — anonymous                          |
| 8883 | TLS     | broker presents cert; client is anonymous |
| 8884 | mTLS    | broker AND client must present a cert     |

All three listeners share the same persistence and address space, so a
publish on `1883` is visible to a subscriber on `8884` (handy for
checking that "encrypted" really only refers to the wire, not the data).

## Bundled certificates

`fixtures/` ships:

```
fixtures/
├── ca/     ca.pem, ca.key       # demo root CA
├── server/ server.pem, server.key   # broker cert (used on 8883/8884)
└── client/ client.pem, client.key   # mTLS client cert (signed by CA)
```

All keys are RSA-2048 in PKCS#1 format (`-----BEGIN RSA PRIVATE KEY-----`)
and certificates are valid for 10 years. Regenerate with:

```bash
bash demo/mqtt-broker/fixtures/regenerate.sh
docker compose restart            # broker re-reads on start
```

> ⚠ The bundled keys are public. Treat the certs as demo fixtures only —
> never use them against any real broker.

## Wiring up a LOOPZE flow

### 1. Plain (port 1883)

`mqtt-broker` config node:

| Field      | Value                |
| ---------- | -------------------- |
| Host       | `localhost`          |
| Port       | `1883`               |
| TLS        | disabled (no `tls` block) |

### 2. Server-only TLS (port 8883)

In LOOPZE → **Certificates** → **+ New certificate**:

- Type `ca-bundle`, Source `file`
- Cert path: `<repo-root>/demo/mqtt-broker/fixtures/ca/ca.pem`
- ID: `mqtt-demo-ca`

`mqtt-broker` config node:

| Field        | Value           |
| ------------ | --------------- |
| Host         | `localhost`     |
| Port         | `8883`          |
| `tls.enabled`     | `true`     |
| `tls.serverName`  | `localhost` (or `loopze-demo-broker`) |
| `tls.caBundleRef` | `mqtt-demo-ca` |

The broker presents its cert; LOOPZE verifies it against the ref'd CA.

### 3. mTLS (port 8884)

Same `mqtt-demo-ca` from step 2, plus a client-pair entry:

- Type `client-pair`, Source `file`
- Cert path: `<repo-root>/demo/mqtt-broker/fixtures/client/client.pem`
- Key path:  `<repo-root>/demo/mqtt-broker/fixtures/client/client.key`
- ID: `mqtt-demo-client`

`mqtt-broker` config node:

| Field           | Value             |
| --------------- | ----------------- |
| Host            | `localhost`       |
| Port            | `8884`            |
| `tls.enabled`        | `true`         |
| `tls.serverName`     | `localhost`    |
| `tls.caBundleRef`    | `mqtt-demo-ca` |
| `tls.clientPairRef`  | `mqtt-demo-client` |

The broker is configured with `use_identity_as_username true`, so the
session shows up under the cert's CN (`loopze-demo-mqtt-client`) — useful
for debugging in `docker compose logs`.

## Smoke test from the command line

A standalone `mosquitto_pub` / `mosquitto_sub` (Linux: `apt install
mosquitto-clients`, macOS: `brew install mosquitto`) makes it easy to
verify the broker before pointing LOOPZE at it.

```bash
# 1883 — plain
mosquitto_sub -h localhost -p 1883 -t demo/#

# 8883 — TLS
mosquitto_sub \
  -h localhost -p 8883 \
  --cafile  fixtures/ca/ca.pem \
  -t demo/#

# 8884 — mTLS
mosquitto_sub \
  -h localhost -p 8884 \
  --cafile  fixtures/ca/ca.pem \
  --cert    fixtures/client/client.pem \
  --key     fixtures/client/client.key \
  -t demo/#
```

## Troubleshooting

- **"Connection refused" on 8883 / 8884**
  Verify the broker is up: `docker compose ps`. If it isn't, the most
  common cause is missing certificate files — run
  `bash fixtures/regenerate.sh`.

- **TLS handshake fails with "x509: certificate signed by unknown authority"**
  LOOPZE's `caBundleRef` must point at the bundled CA (`fixtures/ca/ca.pem`),
  not at the server cert.

- **mTLS rejects the client**
  The cert must be signed by the same CA the broker trusts. Either use
  the bundled `client.pem` or sign your own with `fixtures/ca/ca.pem`.

- **Port already in use**
  Another local broker is on 1883. Edit `docker-compose.yml` to remap
  e.g. `"11883:1883"` and update the LOOPZE config accordingly.
