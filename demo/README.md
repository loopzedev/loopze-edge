# LOOPZE Demo Servers

Sammlung externer Demo-Server zum Testen der LOOPZE-Nodes gegen reale Protokolle.

## Verfügbar

| Folder            | Zweck                                                        |
| ----------------- | ------------------------------------------------------------ |
| `opcua-server/`   | Node.js OPC UA Server: Security, ExtensionObjects, Sim-Daten |
| `modbus-server/`  | Go Modbus TCP Slave: alle FCs, animierte Demo-Werte          |

## Geplant

- `mqtt-broker/`   – Mosquitto + Demo-Topics
- `http-mock/`     – REST-Mock für HTTP-Nodes

Jeder Demo-Server lebt in einem eigenen Subfolder mit eigener `README.md`,
eigenem Build/Tooling und – wo sinnvoll – einem `Dockerfile`. Sie sind
**nicht** Teil des LOOPZE-Hauptbinaries und werden separat gestartet.
