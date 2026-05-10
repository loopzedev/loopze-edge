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

### DB2 (600 bytes) — deterministic ramp

| Address     | Type | Content                                                |
| ----------- | ---- | ------------------------------------------------------ |
| `DB2.DBB0..599` | byte | `buf[i] = i & 0xFF` — ramp for block-mode split tests |

The 600-byte size deliberately exceeds the typical 462-byte PDU payload, so a
single `s7-read` block-mode fetch of all 600 bytes forces the gos7 client to
issue ≥2 `AGReadDB` calls and concatenate them. The deterministic ramp lets
tests verify byte-perfect concatenation across the chunk boundary.

### DB3 (256 bytes) — datatype showcase

One well-known value per TIA-Portal datatype, parked at a stable byte offset
so tests and manual UI checks can assert specific bytes. All multi-byte
fields are big-endian (the canonical S7 wire order). Mirrors the type table
in `specifications/issues/NODE_S7.md`.

| Address              | TIA type | Width | Sample value |
| -------------------- | -------- | ----- | ------------ |
| `DB3.DBX0.0`         | BOOL     | 1 b   | `true`       |
| `DB3.DBX0.1`         | BOOL     | 1 b   | `false`      |
| `DB3.DBX0.7`         | BOOL     | 1 b   | `true`       |
| `DB3.DBB1`           | BYTE     | 1 B   | `0x42` (66)  |
| `DB3.DBB2`           | SINT     | 1 B   | `-42`        |
| `DB3.DBB3`           | USINT    | 1 B   | `200`        |
| `DB3.DBW4`           | WORD     | 2 B   | `0xCAFE`     |
| `DB3.DBW6`           | INT      | 2 B   | `-12345`     |
| `DB3.DBW8`           | UINT     | 2 B   | `50000`      |
| `DB3.DBD10`          | DWORD    | 4 B   | `0xDEADBEEF` |
| `DB3.DBD14`          | DINT     | 4 B   | `-1234567890` |
| `DB3.DBD18`          | UDINT    | 4 B   | `4000000000` |
| `DB3.DBD22`          | REAL     | 4 B   | `3.14159`    |
| `DB3.DBL26`          | LWORD    | 8 B   | `0xFEEDFACECAFEBEEF` |
| `DB3.DBL34`          | LINT     | 8 B   | `-1_234_567_890_123_456` |
| `DB3.DBL42`          | ULINT    | 8 B   | `18_000_000_000_000_000_000` |
| `DB3.DBL50`          | LREAL    | 8 B   | `2.718281828459045` (≈ e) |
| `DB3.DBB58`          | CHAR     | 1 B   | `'A'`        |
| `DB3.DBW59`          | WCHAR    | 2 B   | `'Ω'` (U+03A9) |
| `DB3.STRING61.20`    | STRING   | 22 B  | `"Hello S7"` |
| `DB3.WSTRING83.20`   | WSTRING  | 44 B  | `"Hallo Welt"` |
| `DB3.DBW127`         | S5TIME   | 2 B   | `S5T#5s` (timebase 100ms, value 50) |
| `DB3.DBD129`         | TIME     | 4 B   | `T#1d2h3m4s567ms` (= 93_784_567 ms) |
| `DB3.DBL133`         | LTIME    | 8 B   | `LT#1d2h3m4s567ms890us123ns` |
| `DB3.DBW141`         | DATE     | 2 B   | `D#2026-05-10` (= 13_278 days since 1990-01-01) |
| `DB3.DBD143`         | TOD      | 4 B   | `TOD#12:34:56.789` (= 45_296_789 ms) |
| `DB3.DBL147`         | LTOD     | 8 B   | `LTOD#12:34:56.123_456_789` |
| `DB3.DBL155`         | DT       | 8 B   | `DT#2026-05-10-12:34:56.789` (BCD encoded) |
| `DB3.DBL163`         | LDT      | 8 B   | `LDT#2026-05-10T12:34:56.789Z` (ns since 1970-01-01) |
| `DB3.DTL171`         | DTL      | 12 B  | `2026-05-10 12:34:56.789_000_000` (structured) |
| `DB3.DBB183..255`    | reserved | 73 B  | zero — slack for future types       |

> **Note on `DBL` / `WSTRING` syntax**: `DBL<byte>` is LOOPZE's extension for
> 8-byte wire access on S7-1500 64-bit types; `WSTRING<byte>.<maxlen>` is
> analogous to `STRING` with UCS-2 chars (4-byte header + 2 bytes/char). See
> the `Address Syntax` table in the spec for details.

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

## Compatibility patches (`main.py`)

`python-snap7` 1.4's pure-Python server has two protocol gaps that break
real S7 clients (`gos7`, Snap7-class tooling). `main.py` carries minimal
monkey-patches; both are documented inline at the patch site.

1. **COTP CC frame length** — the upstream server emits a minimal 11-byte
   CC response (TPKT + 7-byte COTP); Snap7 / gos7 expect the canonical
   18-byte body with TPDU size + Calling/Called TSAP parameters (22-byte
   total frame). Without the patch every connect aborts with `invalid PDU
   received` on the gos7 side. Patched method:
   `ServerISOConnection._build_cotp_cc`.
2. **Multi-item read** — `_handle_read_area` only parses the first item
   spec from a multi-read request and hardcodes `item_count = 1` in the
   response. gos7's `AGReadMulti` rejects the mismatch with `invalid CPU
   answer`, breaking any read of two or more variables in one PDU. The
   patch re-parses all N items from `raw_parameters`, reads each from the
   in-memory area and emits a properly formed multi-item response.

Both patches are scoped to the demo and filed for upstream contribution
([gijzelaerr/python-snap7](https://github.com/gijzelaerr/python-snap7)).
The single-item write path in upstream is correct; multi-write is not yet
patched in `main.py` (no test fixture exercises it — added on demand).

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
