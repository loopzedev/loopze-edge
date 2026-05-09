# LOOPZE Demo Servers

Collection of external demo servers for testing the LOOPZE nodes against real protocols.

## Available

| Folder            | Purpose                                                       |
| ----------------- | ------------------------------------------------------------- |
| `opcua-server/`   | Node.js OPC UA server: security, ExtensionObjects, sim data   |
| `modbus-server/`  | Go Modbus TCP slave: all FCs, animated demo values            |
| `mqtt-broker/`    | Mosquitto with three listeners: plain / TLS / mTLS, bundled CA |

## Planned

- `http-mock/`     – REST mock for HTTP nodes

Each demo server lives in its own subfolder with its own `README.md`,
its own build/tooling and – where it makes sense – a `Dockerfile`. They are
**not** part of the main LOOPZE binary and are started separately.
