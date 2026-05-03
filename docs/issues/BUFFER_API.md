# Issue: Buffer API — Byte-Manipulation fuer Function Node und Backend-Connectoren

## Status: Open

## Problembeschreibung

In der industriellen Automatisierung kommunizieren SPSen, Modbus-Geraete und OPC-UA-Server ueber **Byte-Arrays**. Temperaturwerte stecken als Float32 in Register 0-1, Druckwerte als UInt16 in Register 2, Statusbits in einzelnen Bytes. Node.js bietet dafuer die `Buffer`-Klasse — in Node-RED ist sie allgegenwaertig.

LOOPZE nutzt Goja (Go-native JS Engine) fuer den Function Node. Goja hat kein `Buffer`. Ohne Buffer-API koennen Anwender keine Byte-Daten aus Industrie-Connectoren (Modbus, S7, OPC-UA) im Function Node verarbeiten.

## Architektur: Shared Go Package + JS Wrapper

Die Buffer-Implementierung lebt als **Go Package `internal/buffer`** und wird an zwei Stellen exponiert:

```
internal/buffer/buffer.go              ← Go API (eine Implementation)
         │
         ├─→ internal/nodes/function.go    ← JS-Global "Buffer" via Goja Wrapper
         │     var buf = Buffer.from(msg.payload);
         │     msg.temp = buf.readFloatBE(0);
         │
         └─→ internal/nodes/modbus_read.go ← Go-Nodes nutzen buffer.Buffer direkt
               buf := buffer.From(responseBytes)
               temp := buf.ReadFloatBE(0)
```

**Vorteil**: Eine Implementation, zwei Zugangswege. Gleiche Methoden, gleiche Byte-Reihenfolge, gleiche Ergebnisse — egal ob Go oder JS. Verhindert subtile Endianness-Bugs zwischen Connector und Function Node.

## Anforderungen

### 1. Go Package `internal/buffer`

Struct `Buffer` mit `[]byte` als Backing Store. Alle Methoden arbeiten auf dem selben Slice (kein Kopieren bei Read).

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

Intern nutzt das Package `encoding/binary` (BigEndian/LittleEndian) und `math` (Float32frombits, Float64frombits) — keine externen Dependencies.

### 2. JS API im Function Node

Der Function Node registriert `Buffer` als Global in der Goja VM. Die API ist **kompatibel mit Node.js Buffer**, sodass bestehende Node-RED Snippets uebernommen werden koennen.

#### 2.1 Erstellen

```javascript
var buf = Buffer.alloc(10);                    // 10 Bytes, mit 0 gefuellt
var buf = Buffer.from([0x48, 0x65, 0x6C]);     // aus Byte-Array
var buf = Buffer.from("Hello");                // aus String (UTF-8)
var buf = Buffer.from("48656c6c6f", "hex");    // aus Hex-String
var buf = Buffer.from("SGVsbG8=", "base64");   // aus Base64
Buffer.concat([buf1, buf2]);                   // zusammenfuegen
```

#### 2.2 Properties

```javascript
buf.length;    // Anzahl Bytes (read-only)
```

#### 2.3 Lesen — Integer

| Methode | Bytes | Vorzeichen | Endianness |
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

#### 2.4 Lesen — Gleitkomma

| Methode | Bytes | Typ | Endianness |
|---|---|---|---|
| `buf.readFloatBE([offset])` | 4 | IEEE 754 float32 | Big Endian |
| `buf.readFloatLE([offset])` | 4 | IEEE 754 float32 | Little Endian |
| `buf.readDoubleBE([offset])` | 8 | IEEE 754 float64 | Big Endian |
| `buf.readDoubleLE([offset])` | 8 | IEEE 754 float64 | Little Endian |

#### 2.5 Schreiben — Integer

| Methode | Bytes | Vorzeichen | Endianness |
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

#### 2.6 Schreiben — Gleitkomma

| Methode | Bytes | Typ | Endianness |
|---|---|---|---|
| `buf.writeFloatBE(value[, offset])` | 4 | IEEE 754 float32 | Big Endian |
| `buf.writeFloatLE(value[, offset])` | 4 | IEEE 754 float32 | Little Endian |
| `buf.writeDoubleBE(value[, offset])` | 8 | IEEE 754 float64 | Big Endian |
| `buf.writeDoubleLE(value[, offset])` | 8 | IEEE 754 float64 | Little Endian |

#### 2.7 Byte-Swap

| Methode | Beschreibung |
|---|---|
| `buf.swap16()` | Tauscht Byte-Reihenfolge in 16-Bit Paaren (ABCD → BADC) |
| `buf.swap32()` | Tauscht Byte-Reihenfolge in 32-Bit Gruppen (ABCD → DCBA) |
| `buf.swap64()` | Tauscht Byte-Reihenfolge in 64-Bit Gruppen |

Swap-Methoden sind kritisch fuer Modbus-Geraete die "mid-endian" (CDAB) Byte-Reihenfolge verwenden — ein haeufiges Problem in der Praxis.

#### 2.8 Konvertieren

```javascript
buf.toString()            // → UTF-8 String
buf.toString("hex")       // → "48656c6c6f"
buf.toString("base64")    // → "SGVsbG8="
buf.toJSON()              // → [72, 101, 108, 108, 111]
buf.slice(start, end)     // → neuer Buffer (Kopie)
buf.copy(target[, targetStart[, sourceStart[, sourceEnd]]])
```

### 3. Praxisbeispiel: Modbus Register parsen

```javascript
// SPS liefert 8 Bytes aus Holding Registers 0-3
var buf = Buffer.from(msg.payload);

msg.payload = {
    temperature: buf.readFloatBE(0),     // Register 0-1: Temperatur (°C)
    pressure:    buf.readUInt16BE(4),     // Register 2: Druck (mbar)
    status:      buf.readUInt8(6),        // Register 3 high byte: Status
    errorCode:   buf.readUInt8(7),        // Register 3 low byte: Fehlercode
};

return msg;
```

### 4. Praxisbeispiel: Steuerbefehl an SPS senden

```javascript
// 6 Bytes Steuerbefehl zusammenbauen
var buf = Buffer.alloc(6);

buf.writeUInt16BE(msg.payload.setpoint, 0);  // Register 0: Sollwert
buf.writeUInt16BE(msg.payload.speed, 2);     // Register 1: Drehzahl
buf.writeUInt8(msg.payload.mode, 4);         // Register 2 high: Betriebsart
buf.writeUInt8(msg.payload.command, 5);      // Register 2 low: Kommando

msg.payload = buf.toJSON();  // als Byte-Array weiterleiten
return msg;
```

## Betroffene Dateien

### Neue Dateien

- `internal/buffer/buffer.go` — Go Buffer Implementation mit allen Read/Write/Swap/Convert Methoden
- `internal/buffer/buffer_test.go` — Umfangreiche Tests inkl. Endianness-Verifikation

### Geaenderte Dateien

- `internal/nodes/function.go` — `registerGlobals()` erweitern: `Buffer` Objekt mit `alloc`, `from`, `concat` als Static Methods und alle Instanz-Methoden auf dem Goja-Prototype registrieren

## Technische Hinweise

### Go-Implementation

Alle Read/Write Methoden nutzen `encoding/binary`:

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
    // ... alle Methoden
    return obj
}
```

### Bounds Checking

Alle Read/Write Methoden muessen den Offset pruefen und einen JS-Error werfen wenn out-of-bounds:

```go
func (b *Buffer) ReadUInt16BE(offset int) (uint16, error) {
    if offset < 0 || offset+2 > len(b.data) {
        return 0, fmt.Errorf("offset %d out of range [0, %d]", offset, len(b.data)-2)
    }
    return binary.BigEndian.Uint16(b.data[offset:]), nil
}
```

### BigInt64 Handling

Goja unterstuetzt kein natives BigInt. Fuer `readBigInt64BE`/`readBigUInt64BE` gibt es zwei Optionen:
- **Option A**: Als `float64` zurueckgeben (verliert Praezision bei Werten > 2^53)
- **Option B**: Als String zurueckgeben ("`9223372036854775807`")

**Empfehlung: Option A** fuer die meisten Faelle, da 64-Bit Zaehler in der SPS-Welt selten die float64-Grenze ueberschreiten. Dokumentieren dass Praezisionsverlust bei sehr grossen Werten moeglich ist.

## Abhaengigkeiten

- Keine externen Go-Dependencies (nur `encoding/binary`, `encoding/hex`, `encoding/base64`, `math`)
- Wird von zukuenftigen Industrie-Connectoren (Modbus, S7, OPC-UA) direkt als Go-Package genutzt
