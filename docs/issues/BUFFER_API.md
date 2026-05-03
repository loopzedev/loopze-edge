# Issue: Buffer API — Byte Manipulation for Function Node and Backend Connectors

## Status: Open

## Problem Description

In industrial automation, PLCs, Modbus devices, and OPC UA servers communicate via **byte arrays**. Temperature values are stored as Float32 in registers 0-1, pressure values as UInt16 in register 2, status bits in individual bytes. Node.js provides the `Buffer` class for this purpose — in Node-RED it is ubiquitous.

LOOPZE uses Goja (Go-native JS engine) for the Function node. Goja has no `Buffer`. Without a Buffer API, users cannot process byte data from industrial connectors (Modbus, S7, OPC UA) in the Function node.

## Architecture: Shared Go Package + JS Wrapper

The Buffer implementation lives as a **Go package `internal/buffer`** and is exposed in two places:

```
internal/buffer/buffer.go              <- Go API (single implementation)
         |
         +-> internal/nodes/function.go    <- JS global "Buffer" via Goja wrapper
         |     var buf = Buffer.from(msg.payload);
         |     msg.temp = buf.readFloatBE(0);
         |
         +-> internal/nodes/modbus_read.go <- Go nodes use buffer.Buffer directly
               buf := buffer.From(responseBytes)
               temp := buf.ReadFloatBE(0)
```

**Advantage**: One implementation, two access paths. Same methods, same byte order, same results — whether Go or JS. Prevents subtle endianness bugs between connector and Function node.

## Requirements

### 1. Go Package `internal/buffer`

Struct `Buffer` with `[]byte` as backing store. All methods operate on the same slice (no copying on read).

```go
package buffer

type Buffer struct {
    data []byte
}

func Alloc(size int) *Buffer
func From(data []byte) *Buffer
func FromString(s string) *Buffer
func FromHex(hex string) (*Buffer, error)
func FromBase64(b64 string) (*Buffer, error)
func Concat(buffers ...*Buffer) *Buffer
```

Internally the package uses `encoding/binary` (BigEndian/LittleEndian) and `math` (Float32frombits, Float64frombits) — no external dependencies.

### 2. JS API in the Function Node

The Function node registers `Buffer` as a global in the Goja VM. The API is **compatible with Node.js Buffer**, so existing Node-RED snippets can be reused.

#### 2.1 Creating

```javascript
var buf = Buffer.alloc(10);                    // 10 bytes, filled with 0
var buf = Buffer.from([0x48, 0x65, 0x6C]);     // from byte array
var buf = Buffer.from("Hello");                // from string (UTF-8)
var buf = Buffer.from("48656c6c6f", "hex");    // from hex string
var buf = Buffer.from("SGVsbG8=", "base64");   // from base64
Buffer.concat([buf1, buf2]);                   // concatenate
```

#### 2.2 Properties

```javascript
buf.length;    // number of bytes (read-only)
```

#### 2.3 Read — Integer

| Method | Bytes | Sign | Endianness |
|---|---|---|---|
| `buf.readUInt8([offset])` | 1 | unsigned | — |
| `buf.readInt8([offset])` | 1 | signed | — |
| `buf.readUInt16BE([offset])` | 2 | unsigned | Big Endian |
| `buf.readUInt16LE([offset])` | 2 | unsigned | Little Endian |
| `buf.readInt16BE([offset])` | 2 | signed | Big Endian |
| `buf.readInt16LE([offset])` | 2 | signed | Little Endian |
| `buf.readUInt32BE([offset])` | 4 | unsigned | Big Endian |
| `buf.readUInt32LE([offset])` | 4 | unsigned | Little Endian |
| `buf.readInt32BE([offset])` | 4 | signed | Big Endian |
| `buf.readInt32LE([offset])` | 4 | signed | Little Endian |
| `buf.readBigUInt64BE([offset])` | 8 | unsigned | Big Endian |
| `buf.readBigUInt64LE([offset])` | 8 | unsigned | Little Endian |
| `buf.readBigInt64BE([offset])` | 8 | signed | Big Endian |
| `buf.readBigInt64LE([offset])` | 8 | signed | Little Endian |
| `buf.readUIntBE(offset, byteLength)` | 1-6 | unsigned | Big Endian |
| `buf.readUIntLE(offset, byteLength)` | 1-6 | unsigned | Little Endian |
| `buf.readIntBE(offset, byteLength)` | 1-6 | signed | Big Endian |
| `buf.readIntLE(offset, byteLength)` | 1-6 | signed | Little Endian |

#### 2.4 Read — Floating Point

| Method | Bytes | Type | Endianness |
|---|---|---|---|
| `buf.readFloatBE([offset])` | 4 | IEEE 754 float32 | Big Endian |
| `buf.readFloatLE([offset])` | 4 | IEEE 754 float32 | Little Endian |
| `buf.readDoubleBE([offset])` | 8 | IEEE 754 float64 | Big Endian |
| `buf.readDoubleLE([offset])` | 8 | IEEE 754 float64 | Little Endian |

#### 2.5 Write — Integer

| Method | Bytes | Sign | Endianness |
|---|---|---|---|
| `buf.writeUInt8(value[, offset])` | 1 | unsigned | — |
| `buf.writeInt8(value[, offset])` | 1 | signed | — |
| `buf.writeUInt16BE(value[, offset])` | 2 | unsigned | Big Endian |
| `buf.writeUInt16LE(value[, offset])` | 2 | unsigned | Little Endian |
| `buf.writeInt16BE(value[, offset])` | 2 | signed | Big Endian |
| `buf.writeInt16LE(value[, offset])` | 2 | signed | Little Endian |
| `buf.writeUInt32BE(value[, offset])` | 4 | unsigned | Big Endian |
| `buf.writeUInt32LE(value[, offset])` | 4 | unsigned | Little Endian |
| `buf.writeInt32BE(value[, offset])` | 4 | signed | Big Endian |
| `buf.writeInt32LE(value[, offset])` | 4 | signed | Little Endian |
| `buf.writeBigUInt64BE(value[, offset])` | 8 | unsigned | Big Endian |
| `buf.writeBigUInt64LE(value[, offset])` | 8 | unsigned | Little Endian |
| `buf.writeBigInt64BE(value[, offset])` | 8 | signed | Big Endian |
| `buf.writeBigInt64LE(value[, offset])` | 8 | signed | Little Endian |
| `buf.writeUIntBE(value, offset, byteLength)` | 1-6 | unsigned | Big Endian |
| `buf.writeUIntLE(value, offset, byteLength)` | 1-6 | unsigned | Little Endian |
| `buf.writeIntBE(value, offset, byteLength)` | 1-6 | signed | Big Endian |
| `buf.writeIntLE(value, offset, byteLength)` | 1-6 | signed | Little Endian |

#### 2.6 Write — Floating Point

| Method | Bytes | Type | Endianness |
|---|---|---|---|
| `buf.writeFloatBE(value[, offset])` | 4 | IEEE 754 float32 | Big Endian |
| `buf.writeFloatLE(value[, offset])` | 4 | IEEE 754 float32 | Little Endian |
| `buf.writeDoubleBE(value[, offset])` | 8 | IEEE 754 float64 | Big Endian |
| `buf.writeDoubleLE(value[, offset])` | 8 | IEEE 754 float64 | Little Endian |

#### 2.7 Byte Swap

| Method | Description |
|---|---|
| `buf.swap16()` | Swaps byte order in 16-bit pairs (ABCD -> BADC) |
| `buf.swap32()` | Swaps byte order in 32-bit groups (ABCD -> DCBA) |
| `buf.swap64()` | Swaps byte order in 64-bit groups |

Swap methods are critical for Modbus devices that use "mid-endian" (CDAB) byte order — a common real-world problem.

#### 2.8 Convert

```javascript
buf.toString()            // -> UTF-8 string
buf.toString("hex")       // -> "48656c6c6f"
buf.toString("base64")    // -> "SGVsbG8="
buf.toJSON()              // -> [72, 101, 108, 108, 111]
buf.slice(start, end)     // -> new Buffer (copy)
buf.copy(target[, targetStart[, sourceStart[, sourceEnd]]])
```

### 3. Real-World Example: Parse Modbus Registers

```javascript
// PLC delivers 8 bytes from holding registers 0-3
var buf = Buffer.from(msg.payload);

msg.payload = {
    temperature: buf.readFloatBE(0),     // register 0-1: temperature (degC)
    pressure:    buf.readUInt16BE(4),     // register 2: pressure (mbar)
    status:      buf.readUInt8(6),        // register 3 high byte: status
    errorCode:   buf.readUInt8(7),        // register 3 low byte: error code
};

return msg;
```

### 4. Real-World Example: Send Control Command to PLC

```javascript
// assemble 6-byte control command
var buf = Buffer.alloc(6);

buf.writeUInt16BE(msg.payload.setpoint, 0);  // register 0: setpoint
buf.writeUInt16BE(msg.payload.speed, 2);     // register 1: speed
buf.writeUInt8(msg.payload.mode, 4);         // register 2 high: operating mode
buf.writeUInt8(msg.payload.command, 5);      // register 2 low: command

msg.payload = buf.toJSON();  // forward as byte array
return msg;
```

## Affected Files

### New Files

- `internal/buffer/buffer.go` — Go Buffer implementation with all read/write/swap/convert methods
- `internal/buffer/buffer_test.go` — Comprehensive tests including endianness verification

### Changed Files

- `internal/nodes/function.go` — extend `registerGlobals()`: register `Buffer` object with `alloc`, `from`, `concat` as static methods and all instance methods on the Goja prototype

## Technical Notes

### Go Implementation

All read/write methods use `encoding/binary`:

```go
func (b *Buffer) ReadUInt16BE(offset int) uint16 {
    return binary.BigEndian.Uint16(b.data[offset:])
}

func (b *Buffer) ReadFloatBE(offset int) float32 {
    bits := binary.BigEndian.Uint32(b.data[offset:])
    return math.Float32frombits(bits)
}

func (b *Buffer) WriteFloatBE(value float32, offset int) {
    bits := math.Float32bits(value)
    binary.BigEndian.PutUint32(b.data[offset:], bits)
}
```

### Goja Wrapper Pattern

```go
func (n *FunctionNode) registerBuffer() {
    bufferCtor := n.vm.NewObject()

    // Buffer.alloc(size)
    _ = bufferCtor.Set("alloc", func(call goja.FunctionCall) goja.Value {
        size := int(call.Argument(0).ToInteger())
        return n.wrapBuffer(buffer.Alloc(size))
    })

    // Buffer.from(data, encoding?)
    _ = bufferCtor.Set("from", func(call goja.FunctionCall) goja.Value {
        // ... parse array, string, hex, base64
        return n.wrapBuffer(buf)
    })

    _ = n.vm.Set("Buffer", bufferCtor)
}

func (n *FunctionNode) wrapBuffer(buf *buffer.Buffer) goja.Value {
    obj := n.vm.NewObject()
    _ = obj.Set("length", buf.Length())
    _ = obj.Set("readUInt8", func(call goja.FunctionCall) goja.Value { ... })
    _ = obj.Set("readUInt16BE", func(call goja.FunctionCall) goja.Value { ... })
    // ... all methods
    return obj
}
```

### Bounds Checking

All read/write methods must check the offset and throw a JS error when out-of-bounds:

```go
func (b *Buffer) ReadUInt16BE(offset int) (uint16, error) {
    if offset < 0 || offset+2 > len(b.data) {
        return 0, fmt.Errorf("offset %d out of range [0, %d]", offset, len(b.data)-2)
    }
    return binary.BigEndian.Uint16(b.data[offset:]), nil
}
```

### BigInt64 Handling

Goja does not support native BigInt. For `readBigInt64BE`/`readBigUInt64BE` there are two options:
- **Option A**: return as `float64` (loses precision for values > 2^53)
- **Option B**: return as string ("`9223372036854775807`")

**Recommendation: Option A** for most cases, since 64-bit counters in the PLC world rarely exceed the float64 limit. Document that precision loss is possible for very large values.

## Dependencies

- No external Go dependencies (only `encoding/binary`, `encoding/hex`, `encoding/base64`, `math`)
- Will be used directly as a Go package by future industrial connectors (Modbus, S7, OPC UA)
