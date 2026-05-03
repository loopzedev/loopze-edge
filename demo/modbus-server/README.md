# LOOPZE Modbus Demo Server

A lean Modbus TCP slave (Go, no external dependencies) for testing the
LOOPZE Modbus nodes (`modbus-server` config, `modbus-read`, `modbus-write`).

It populates its register spaces with a few defined values and animates
some of them so polling clients see movement.

## Quick start

```bash
make demo-modbus            # runs on :5502 with request trace
# or directly:
go run ./demo/modbus-server -listen :5502 -v
```

Options:

| Flag        | Default  | Meaning                                             |
| ----------- | -------- | --------------------------------------------------- |
| `-listen`   | `:5502`  | TCP listen address (e.g. `:502`, `127.0.0.1:1502`)  |
| `-v`        | `false`  | Logs every incoming Modbus request                  |

Port `502` is privileged — either start with `sudo`, set
`setcap cap_net_bind_service=+ep` on the binary, or simply stick with the
default `:5502` and enter port `5502` in the LOOPZE server config.

## Address map

All values are encoded with **big-endian byte order** and **big-endian word
order** (ABCD) — i.e. the Modbus default. To test the codec against all four
order combinations, simply switch to little-byte or little-word in the LOOPZE
client and compare.

### Holding Registers (FC3, RW)

| Address  | Type     | Content                                                  |
| -------- | -------- | -------------------------------------------------------- |
| 0..1     | float32  | Temperature (°C) — drifts slowly between 18 and 24       |
| 2..3     | uint32   | Tick counter — counts 4 times per second                 |
| 4..5     | float32  | Pressure (bar) — random walk around 1.0 bar              |
| 6        | int16    | Setpoint — RW, default 200                               |
| 7        | uint16   | Mode — RW, default 1                                     |
| 10..14   | string   | "LOOPZE-DEMO" (5 registers, 10 ASCII characters)         |
| 20..21   | float32  | Energy (kWh) — monotonically increasing, 1 kWh / minute  |

### Input Registers (FC4, RO)

| Address  | Type     | Content                                  |
| -------- | -------- | ---------------------------------------- |
| 0..1     | float32  | Live sensor — pure 0.5 Hz sine, ±1.0     |
| 2        | uint16   | RPM — random walk in [1200, 1800]        |

### Coils (FC1, RW)

Addresses 0..63 are all writable, default `false`. FC5 (single) and FC15
(multiple) both work.

### Discrete Inputs (FC2, RO)

Addresses 0..15 show a **running light**: every second a set bit moves one
position further. Practical for seeing that polling actually fetches fresh
data.

## Verify

In LOOPZE:

1. Create a config node `Modbus Server`, `host=127.0.0.1`, `port=5502`,
   `defaultUnitId=1`.
2. Drag a `Modbus Read` onto the canvas, select the server.
3. FC3, address 0, dataType float32, polling 1 s — attach a Debug node.

In the Debug panel a temperature on the order of 21 °C should arrive,
slowly changing.

## Example: Read raw + decode to float32 in a Function node

Instead of letting the codec in the Read node handle it, you can fetch the
raw registers and parse them in a Function node with the Buffer API. This is
practical, for example, when a device delivers several values of different
types in a contiguous block and you want to break them apart in one step.

**Modbus Read** — all defaults from the `Modbus Demo` setup, only:

```
Function Code: FC3 — Read Holding Registers
Address:       0
Quantity:      2
Data Type:     raw          ← raw, both representations are delivered
Polling:       1000 ms
```

For FC3 / FC4 with `dataType: raw` the Read node delivers **both** views in
the same message:

| Field         | Format                  | What for                                    |
| ------------- | ----------------------- | ------------------------------------------- |
| `msg.payload` | `[]int` word array      | Direct access to individual registers       |
| `msg.bytes`   | `[]int` wire bytes      | `Buffer.from(msg.bytes)` for byte parsing   |
| `msg.modbus`  | Metadata (FC, address)  | Debugging / round-trip                      |

This way the Function node picks the view appropriate for the use case
without conversion boilerplate.

**Function node** — float32 from wire bytes:

```javascript
// msg.bytes is exactly what came from the bus (4 bytes for 2 registers).
const buf = Buffer.from(msg.bytes);

msg.payload = buf.readFloatBE(0);   // ≈ 21.5 °C
msg.topic   = 'temperature';
return msg;
```

**Alternative** — same task via the word array (e.g. when you want to decode
each register differently):

```javascript
// msg.payload = [reg0, reg1] as uint16 — assemble bytes yourself.
const buf = Buffer.alloc(4);
buf.writeUInt16BE(msg.payload[0], 0);
buf.writeUInt16BE(msg.payload[1], 2);

msg.payload = buf.readFloatBE(0);
return msg;
```

**A ready-made flow** is available as `example-flow.json` next to this
README — importable via *Import* in the editor toolbar.

> **Tip** for CDAB / BADC / DCBA devices: call `buf.swap16()` or
> `buf.swap32()` on the Buffer object before `readFloatBE` runs. This
> covers all four byte/word order combinations in the Function node
> without touching the Read node.

## Function Codes

Supported: **FC1, FC2, FC3, FC4, FC5, FC6, FC15, FC16**.

Not supported (returns `Illegal Function`, code 0x01): FC7, FC11, FC12,
FC17, FC20–24, FC43.

## Exception Codes

The server returns the standard Modbus exceptions:

| Code   | Meaning                 | When                                        |
| ------ | ----------------------- | ------------------------------------------- |
| 0x01   | Illegal Function        | unknown / unsupported FC                    |
| 0x02   | Illegal Data Address    | `addr + qty > 65536`                        |
| 0x03   | Illegal Data Value      | `qty` outside the spec-allowed range        |

## Properties / limits

- Accepts **any** unit ID (slave ID) — irrelevant in the demo context
- A mutex serializes all access to the data store (real devices serialize
  per connection anyway)
- Data does **not** persist across restarts — on stop the setpoint is back
  to 200, coils all back to `false`
- Address space limits are the Modbus spec maxima: 2000 coils/discretes per
  read, 125 registers per read, 1968 coils per write, 123 registers per write
