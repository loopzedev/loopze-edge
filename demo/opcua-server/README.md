# LOOPZE OPC UA Demo Server

A Node.js / `node-opcua` based OPC UA server for testing the LOOPZE
OPC UA nodes (`opcua-server`, `opcua-read`, `opcua-subscribe`, `opcua-write`).

It provides all OPC UA security concepts, Extension Objects (UDTs) and
simulated demo data – exactly what a real PLC / machine would deliver.

## Prerequisites

- Node.js >= 20
- npm

## Quick start

```bash
cd demo/opcua-server
npm install
npm start
```

The server binds by default on:

```
opc.tcp://localhost:4840/loopze-demo
```

On first start a self-signed server certificate is generated automatically
in `pki/server/own/`. The `pki/` directory is `gitignore`d.

## Endpoints / security

The server advertises **all combinations** of the configured SecurityPolicies
× SecurityModes per run. Default (see `config/default.yaml`):

| SecurityPolicy             | SecurityMode             |
| -------------------------- | ------------------------ |
| None                       | None                     |
| Basic128Rsa15              | Sign / SignAndEncrypt    |
| Basic256                   | Sign / SignAndEncrypt    |
| Basic256Sha256             | Sign / SignAndEncrypt    |
| Aes128_Sha256_RsaOaep      | Sign / SignAndEncrypt    |
| Aes256_Sha256_RsaPss       | Sign / SignAndEncrypt    |

This way **one** LOOPZE configuration can be tested through all variants
without changing the server.

## Authentication

| Mode               | Credentials                 |
| ------------------ | --------------------------- |
| Anonymous          | (no login)                  |
| Username/Password  | see `config/users.json`     |
| X.509 Certificate  | cert in `pki/user/trusted/` |

Default users (demo, **not** for production!):

| Username  | Password  | Role     |
| --------- | --------- | -------- |
| operator  | operator  | Operator |
| engineer  | engineer  | Engineer |
| admin     | admin     | Admin    |

The demo roles map onto the OPC UA WellKnownRoles (`Operator`, `Engineer`,
`AuthenticatedUser`, …).

## Address space

All demo data lives in the namespace `urn:loopze:demo` (namespace index `2`).

```
Objects/
└── Demo/
    ├── Static/
    │   ├── Scalar/                # Boolean, SByte, Byte, Int16/32/64, UInt..,
    │   │                          # Float, Double, String, DateTime, ByteString,
    │   │                          # Guid, LocalizedText
    │   └── Array/                 # Int32Array, DoubleArray, StringArray,
    │                              # ByteArrayAsByteString
    ├── Dynamic/                   # live updated every tick
    │   ├── Counter        Int32  – +1/sec
    │   ├── Sine           Double – 0.5 Hz sine
    │   ├── Sawtooth       Double – sawtooth 0..1
    │   ├── Random         Double – Math.random()
    │   ├── Temperature    Double – ~20 °C with drift + noise
    │   └── Pressure       Double – 1.013 bar with spikes (good for deadband)
    ├── Structures/                # Extension Objects / UDTs
    │   ├── MotorStatus    MotorStatusType    (live)
    │   ├── SensorReading  SensorReadingType
    │   └── Recipe         RecipeType  (nested + array of steps)
    └── Writable/                  # for opcua-write tests
        ├── Setpoint       Double
        ├── Mode           Int32  (0=Manual 1=Auto 2=Service)
        ├── Command        String
        └── MotorCmd       MotorCommandType  (ExtensionObject write)
```

### UDT definitions

| Type                | Fields |
| ------------------- | ------ |
| `MotorStatusType`   | Speed (Double), Torque (Double), FaultCode (UInt32), Running (Boolean), Mode (String) |
| `SensorReadingType` | Value (Double), Unit (String), Quality (UInt16), Timestamp (DateTime) |
| `MotorCommandType`  | TargetSpeed (Double), AccelRamp (UInt32), Direction (String), EnableLimits (Boolean) |
| `RecipeStepType`    | StepNo (UInt16), Duration (Double), Setpoint (Double) |
| `RecipeType`        | Name (String), Version (UInt32), Steps (Array of `RecipeStepType`) |

Every UDT DataType node carries a `DataTypeDefinition` so LOOPZE's
type resolver detects the fields automatically and can map JSON ↔
ExtensionObject.

## Test scenarios for LOOPZE

### 1. opcua-read · scalars

NodeIDs: `ns=2;s=Demo.Static.Scalar.Double`, `ns=2;s=Demo.Static.Scalar.String`

### 2. opcua-subscribe · dynamic + deadband

NodeID: `ns=2;s=Demo.Dynamic.Pressure` with deadband `absolute 0.1` – spikes
are pushed, small noise is filtered.

### 3. opcua-read · ExtensionObject (read path)

NodeID: `ns=2;s=Demo.Structures.MotorStatus` – LOOPZE should output this as a
JSON object (`{ Speed, Torque, FaultCode, Running, Mode }`) plus
`structureType`, `structureName`.

### 4. opcua-write · ExtensionObject (write path)

NodeID: `ns=2;s=Demo.Writable.MotorCmd` with:

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

The server logs every write to stdout.

### 5. Smoke test with the included verify script

```bash
# Terminal 1: server is running
npm start

# Terminal 2: read all demo nodes incl. ExtensionObjects
npm run verify
```

Returns one line per NodeID with status code and value (UDTs as JSON).

### 6. LOOPZE backend tests

The OPC UA tests in `internal/nodes/opcua_*_test.go` activate when
`LOOPZE_OPCUA_TEST_ENDPOINT` is set:

```bash
# Terminal 1
cd demo/opcua-server && npm start

# Terminal 2
LOOPZE_OPCUA_TEST_ENDPOINT=opc.tcp://localhost:4840/loopze-demo \
  go test ./internal/nodes/...
```

## Configuration

The default configuration is in `config/default.yaml`. Custom config via
env variable:

```bash
LOOPZE_OPCUA_DEMO_CONFIG=/path/to/my-config.yaml npm start
```

Important fields:

- `endpoint.port` – default `4840`
- `security.policies` / `security.modes` – list of offered variants
- `security.allowAnonymous` – `true`/`false`
- `security.autoAcceptUnknownCertificate` – TOFU for client certs
- `simulation.enableDynamicValues` – sim loop on/off
- `simulation.tickIntervalMs` / `simulation.motorTickMs` – sim frequencies

## Docker

```bash
docker build -t loopze-opcua-demo .
docker run --rm -p 4840:4840 loopze-opcua-demo
```

## Troubleshooting

- **"BadCertificateChainIncomplete" when connecting**
  Expected on the first SecurityMode≠None attempt. Client cert lands in
  `pki/server/rejected/` – move to `pki/server/trusted/certs/` (or leave
  `autoAcceptUnknownCertificate: true`).

- **"BadIdentityTokenRejected"**
  Wrong username/password or auth mode does not match the endpoint.

- **Port in use**
  `lsof -i :4840` – an old server process may still be running.

## Other demo servers

Planned in `demo/`:

- `demo/modbus-server/` – Modbus TCP/RTU
- `demo/mqtt-broker/`   – Mosquitto compose with demo topics
- `demo/http-mock/`     – REST mock for HTTP nodes
