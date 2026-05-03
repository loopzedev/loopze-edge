# LOOPZE OPC UA Demo Server

Ein Node.js / `node-opcua` basierter OPC UA Server für das Testen der LOOPZE
OPC-UA-Nodes (`opcua-server`, `opcua-read`, `opcua-subscribe`, `opcua-write`).

Er stellt alle OPC-UA Security-Konzepte, Extension Objects (UDTs) und simulierte
Demo-Daten bereit – also genau das, was eine echte SPS / Maschine liefern würde.

## Voraussetzungen

- Node.js >= 20
- npm

## Schnellstart

```bash
cd demo/opcua-server
npm install
npm start
```

Der Server bindet standardmäßig auf:

```
opc.tcp://localhost:4840/loopze-demo
```

Beim ersten Start wird automatisch ein Self-Signed Server-Zertifikat in
`pki/server/own/` erzeugt. Das `pki/`-Verzeichnis ist `gitignore`d.

## Endpoints / Security

Der Server advertiseiert pro Lauf **alle Kombinationen** der konfigurierten
SecurityPolicies × SecurityModes. Default (siehe `config/default.yaml`):

| SecurityPolicy             | SecurityMode             |
| -------------------------- | ------------------------ |
| None                       | None                     |
| Basic128Rsa15              | Sign / SignAndEncrypt    |
| Basic256                   | Sign / SignAndEncrypt    |
| Basic256Sha256             | Sign / SignAndEncrypt    |
| Aes128_Sha256_RsaOaep      | Sign / SignAndEncrypt    |
| Aes256_Sha256_RsaPss       | Sign / SignAndEncrypt    |

So lässt sich **eine** LOOPZE-Konfiguration durch alle Varianten testen, ohne
den Server umzustellen.

## Authentication

| Mode               | Credentials                 |
| ------------------ | --------------------------- |
| Anonymous          | (kein Login)                |
| Username/Password  | siehe `config/users.json`   |
| X.509 Certificate  | Cert in `pki/user/trusted/` |

Default-User (Demo, **nicht** für Produktion!):

| Username  | Password  | Role     |
| --------- | --------- | -------- |
| operator  | operator  | Operator |
| engineer  | engineer  | Engineer |
| admin     | admin     | Admin    |

Die Demo-Rollen mappen auf die OPC UA WellKnownRoles (`Operator`, `Engineer`,
`AuthenticatedUser`, …).

## Adressraum

Alle Demo-Daten leben im Namespace `urn:loopze:demo` (Namespace-Index `2`).

```
Objects/
└── Demo/
    ├── Static/
    │   ├── Scalar/                # Boolean, SByte, Byte, Int16/32/64, UInt..,
    │   │                          # Float, Double, String, DateTime, ByteString,
    │   │                          # Guid, LocalizedText
    │   └── Array/                 # Int32Array, DoubleArray, StringArray,
    │                              # ByteArrayAsByteString
    ├── Dynamic/                   # live updated jeden Tick
    │   ├── Counter        Int32  – +1/sec
    │   ├── Sine           Double – 0.5 Hz Sinus
    │   ├── Sawtooth       Double – Sägezahn 0..1
    │   ├── Random         Double – Math.random()
    │   ├── Temperature    Double – ~20 °C mit Drift + Rauschen
    │   └── Pressure       Double – 1.013 bar mit Spikes (gut für Deadband)
    ├── Structures/                # Extension Objects / UDTs
    │   ├── MotorStatus    MotorStatusType    (live)
    │   ├── SensorReading  SensorReadingType
    │   └── Recipe         RecipeType  (verschachtelt + Array of Steps)
    └── Writable/                  # für opcua-write Tests
        ├── Setpoint       Double
        ├── Mode           Int32  (0=Manual 1=Auto 2=Service)
        ├── Command        String
        └── MotorCmd       MotorCommandType  (ExtensionObject Write)
```

### UDT-Definitionen

| Type                | Felder |
| ------------------- | ------ |
| `MotorStatusType`   | Speed (Double), Torque (Double), FaultCode (UInt32), Running (Boolean), Mode (String) |
| `SensorReadingType` | Value (Double), Unit (String), Quality (UInt16), Timestamp (DateTime) |
| `MotorCommandType`  | TargetSpeed (Double), AccelRamp (UInt32), Direction (String), EnableLimits (Boolean) |
| `RecipeStepType`    | StepNo (UInt16), Duration (Double), Setpoint (Double) |
| `RecipeType`        | Name (String), Version (UInt32), Steps (Array of `RecipeStepType`) |

Jeder UDT-DataType-Knoten trägt eine `DataTypeDefinition`, sodass LOOPZE's
Type-Resolver die Felder automatisch erkennt und JSON ↔ ExtensionObject
mappen kann.

## Testszenarien für LOOPZE

### 1. opcua-read · Scalars

NodeIDs: `ns=2;s=Demo.Static.Scalar.Double`, `ns=2;s=Demo.Static.Scalar.String`

### 2. opcua-subscribe · Dynamic + Deadband

NodeID: `ns=2;s=Demo.Dynamic.Pressure` mit Deadband `absolute 0.1` – Spikes
werden gepusht, kleines Rauschen wird gefiltert.

### 3. opcua-read · ExtensionObject (Read-Pfad)

NodeID: `ns=2;s=Demo.Structures.MotorStatus` – LOOPZE sollte das als JSON-Objekt
ausgeben (`{ Speed, Torque, FaultCode, Running, Mode }`) plus `structureType`,
`structureName`.

### 4. opcua-write · ExtensionObject (Write-Pfad)

NodeID: `ns=2;s=Demo.Writable.MotorCmd` mit:

```json
{
  "writes": [{
    "nodeId": "ns=2;s=Demo.Writable.MotorCmd",
    "dataType": "ExtensionObject",
    "structureType": "<MotorCommandType-NodeID>",
    "value": {
      "TargetSpeed": 1500,
      "AccelRamp": 200,
      "Direction": "CW",
      "EnableLimits": true
    }
  }]
}
```

Server loggt jeden Write nach stdout.

### 5. Smoke-Test mit dem mitgelieferten Verify-Skript

```bash
# Terminal 1: Server läuft
npm start

# Terminal 2: Read auf alle Demo-Nodes inkl. ExtensionObjects
npm run verify
```

Liefert eine Zeile pro NodeID mit Statuscode und Wert (UDTs als JSON).

### 6. LOOPZE Backend-Tests

Die OPC-UA-Tests in `internal/nodes/opcua_*_test.go` aktivieren sich, wenn
`LOOPZE_OPCUA_TEST_ENDPOINT` gesetzt ist:

```bash
# Terminal 1
cd demo/opcua-server && npm start

# Terminal 2
LOOPZE_OPCUA_TEST_ENDPOINT=opc.tcp://localhost:4840/loopze-demo \
  go test ./internal/nodes/...
```

## Konfiguration

Die Default-Konfiguration ist in `config/default.yaml`. Eigene Konfig per
Env-Variable:

```bash
LOOPZE_OPCUA_DEMO_CONFIG=/path/to/my-config.yaml npm start
```

Wichtige Felder:

- `endpoint.port` – Default `4840`
- `security.policies` / `security.modes` – Liste der angebotenen Varianten
- `security.allowAnonymous` – `true`/`false`
- `security.autoAcceptUnknownCertificate` – TOFU für Client-Certs
- `simulation.enableDynamicValues` – Sim-Loop ein/aus
- `simulation.tickIntervalMs` / `simulation.motorTickMs` – Sim-Frequenzen

## Docker

```bash
docker build -t loopze-opcua-demo .
docker run --rm -p 4840:4840 loopze-opcua-demo
```

## Troubleshooting

- **„BadCertificateChainIncomplete" beim Verbinden**
  Erwartet beim ersten SecurityMode≠None-Versuch. Client-Cert landet in
  `pki/server/rejected/` – nach `pki/server/trusted/certs/` verschieben (oder
  `autoAcceptUnknownCertificate: true` lassen).

- **„BadIdentityTokenRejected"**
  Falscher Username/Passwort oder Auth-Mode passt nicht zum Endpoint.

- **Port belegt**
  `lsof -i :4840` – evtl. läuft noch ein alter Server-Prozess.

## Weitere Demo-Server

Geplant in `demo/`:

- `demo/modbus-server/` – Modbus TCP/RTU
- `demo/mqtt-broker/`   – Mosquitto-Compose mit Demo-Topics
- `demo/http-mock/`     – REST-Mock für HTTP-Nodes
