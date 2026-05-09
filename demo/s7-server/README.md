# LOOPZE S7 Demo PLC

A small SIEMENS S7 server for testing the LOOPZE S7 nodes (`s7-plc` config,
`s7-read`, `s7-write`). Built on `python-snap7`, which since 1.4 ships a
pure-Python S7 server implementation — `pip install python-snap7` brings
everything needed, no system packages, no Docker daemon, no native deps.

It pre-fills DB1 / DB10, the Merker area, and inputs/outputs with well-known
values and animates a handful of "live" measurements so polling clients see
movement.

## Quick start

```bash
make demo-s7                # runs on :1102 with request trace
# or directly:
python3 -m venv .venv && .venv/bin/pip install -r requirements.txt
.venv/bin/python main.py -p 1102 -v
```

Options:

| Flag         | Default | Meaning                                         |
| ------------ | ------- | ----------------------------------------------- |
| `-p`/`--port`| `1102`  | TCP listen port. Use `102` for the standard S7 port — that requires `sudo` or `setcap cap_net_bind_service=+ep` on the python binary. The default `:1102` matches the LOOPZE PLC config in the test fixtures. |
| `-v`/`--verbose` | `false` | Drain the snap7 event queue and log every client request |

## In LOOPZE

1. Create a config node `S7 PLC`:
   - Host `127.0.0.1`, Port `1102`
   - Connection `S7-1200 / S7-1500` (rack=0, slot=1 — auto-derived)
2. Drag an `S7 Read` onto the canvas, select the PLC.
3. Add a variable: address `DB10.DBD0`, dataType `real`, polling 1 s — attach
   a Debug node.

A pure 1 Hz sine wave between -1.0 and 1.0 should show up in the Debug panel.

## Address map

### DB1 (200 bytes) — general sensor data

| Address              | Type   | Content                                          |
| -------------------- | ------ | ------------------------------------------------ |
| `DB1.DBD0`           | REAL   | Temperature (°C) — drifts in [18.0, 24.0]        |
| `DB1.DBD4`           | REAL   | Pressure (bar)  — random walk around 1.0         |
| `DB1.DBD8`           | DINT   | Tick counter    — increments every 250 ms        |
| `DB1.DBW12`          | INT    | Setpoint        — RW; default 200                |
| `DB1.DBW14`          | INT    | Mode            — RW; default 1                  |
| `DB1.DBD16`          | REAL   | Energy (kWh)    — monotonically increasing       |
| `DB1.STRING50.20`    | STRING | "LOOPZE-S7-DEMO" (S7 STRING, max 20 chars)       |

### DB10 (50 bytes) — live measurement

| Address     | Type | Content                                       |
| ----------- | ---- | --------------------------------------------- |
| `DB10.DBD0` | REAL | Sine wave (1 Hz, ±1.0)                        |
| `DB10.DBW4` | INT  | RPM — random walk in [1200, 1800]             |

### M (Merker, 16 bytes)

| Address  | Type  | Content                                            |
| -------- | ----- | -------------------------------------------------- |
| `M0.0`   | BOOL  | Heartbeat — toggles every second                   |
| `M0.1`–`M0.7` | BOOL | Writable scratch flags; default `false`        |
| `MB1`    | BYTE  | Writable; default 0                                |
| `MD4`    | DWORD | Writable counter (operator-driven); default 0      |

### I (Inputs, 16 bytes, RO)

| Address | Type | Content                                                   |
| ------- | ---- | --------------------------------------------------------- |
| `IB0`   | BYTE | Running-light pattern — one bit shifts per second, wraps  |
| `IB1`–`IB15` | BYTE | Static zero                                          |

### Q (Outputs, 16 bytes, RW)

All addresses `Q0.0`–`QB15` are writable; default 0.

## Verify with the standard data types

A Function-node-free sanity flow:

| Variable name | Address       | Type | Expected value                        |
| ------------- | ------------- | ---- | ------------------------------------- |
| `Temperature` | `DB1.DBD0`    | real | ~21 °C, slowly changing               |
| `Tick`        | `DB1.DBD8`    | dint | monotonic counter (+4 / second)       |
| `Heartbeat`   | `M0.0`        | bool | toggles every second                  |
| `RunningBit`  | `IB0`         | byte | rotating one-hot byte                 |
| `Tag`         | `DB1.STRING50.20` | string | `"LOOPZE-S7-DEMO"`                |

Set `output Shape = Object` on `s7-read` and the Debug panel shows them as a
neat dictionary.

## Properties / limits

- The Snap7 server library is a passive data store: it accepts any `rack`/`slot`
  combination the client sends and any valid Connect request — there is no
  authentic CPU-type negotiation.
- `GetCpuInfo` and `GetOrderCode` return Snap7 placeholder strings; the LOOPZE
  Test-Connection endpoint must tolerate these placeholders.
- **Optimized DBs** are not modelled — every registered DB behaves like a
  classical (non-optimized) S7 DB and accepts byte-level addressing. This is
  ideal for testing the LOOPZE address parser and codec; for testing rejection
  of Optimized DBs you need PLCSIM Advanced or real hardware.
- Data does **not** persist across restarts — every run starts with the static
  defaults documented above.
- A single mutex inside `python-snap7` serializes access to each registered
  area, mirroring real S7 PLCs which serialize per connection.
- Memory areas are allocated as fixed-size `ctypes` byte arrays (200 / 50 / 16
  bytes). Reads or writes beyond those bounds return the standard Snap7
  `Address out of range` error, which the LOOPZE error mapping surfaces.

## Why python-snap7 (vs. a Go-native server)

`github.com/robinson/gos7` is client-only — the library does not bundle a
server. The two practical options for a test PLC are:

1. **Snap7 server** (C++) via Docker, or
2. **`python-snap7`**, which since 1.4 includes a pure-Python S7 server
   (no native dependency for the server side; the client side keeps the
   libsnap7 wrapper, but the server is plain Python).

Option 2 wins on tooling overhead: no Docker daemon required, the entire
demo lives in a single Python file that can be edited and re-run instantly,
and the address layout is versioned alongside the code.

## Use from CI / Go tests

The integration tests for `internal/nodes/s7_*` connect via
`LOOPZE_S7_TEST_HOST` / `LOOPZE_S7_TEST_PORT` — when those env vars are unset
the tests skip gracefully (consistent with the OPC UA test pattern). Typical
local invocation:

```bash
make demo-s7 &           # background
LOOPZE_S7_TEST_HOST=127.0.0.1 LOOPZE_S7_TEST_PORT=1102 \
  go test ./internal/nodes -run S7
```
