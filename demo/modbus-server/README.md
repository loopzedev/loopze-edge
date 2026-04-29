# Flint Modbus Demo Server

Ein schlanker Modbus-TCP-Slave (Go, ohne externe Dependencies) zum Testen der
Flint Modbus-Nodes (`modbus-server`-Config, `modbus-read`, `modbus-write`).

Er füllt seine Register-Räume mit ein paar definierten Werten und animiert
einen Teil davon, damit pollende Clients Bewegung sehen.

## Schnellstart

```bash
make demo-modbus            # läuft auf :5502 mit Request-Trace
# oder direkt:
go run ./demo/modbus-server -listen :5502 -v
```

Optionen:

| Flag        | Default  | Bedeutung                                           |
| ----------- | -------- | --------------------------------------------------- |
| `-listen`   | `:5502`  | TCP-Listen-Adresse (z.B. `:502`, `127.0.0.1:1502`) |
| `-v`        | `false`  | Loggt jeden eingehenden Modbus-Request              |

Port `502` ist privileged — entweder mit `sudo` starten, `setcap cap_net_bind_service=+ep`
auf der Binary setzen, oder einfach beim Default `:5502` bleiben und im
Flint-Server-Config Port `5502` eintragen.

## Adressbelegung

Alle Werte sind **big-endian byte order** mit **big-endian word order** (ABCD)
codiert — also der Modbus-Default. Wer den Codec gegen alle vier
Order-Kombinationen testen will, stellt im Flint-Client einfach Little-Byte
oder Little-Word ein und vergleicht.

### Holding Registers (FC3, RW)

| Adresse  | Typ      | Inhalt                                                   |
| -------- | -------- | -------------------------------------------------------- |
| 0..1     | float32  | Temperatur (°C) — driftet langsam zwischen 18 und 24     |
| 2..3     | uint32   | Tick-Counter — zählt 4 mal pro Sekunde                   |
| 4..5     | float32  | Druck (bar) — Random Walk um 1.0 bar                     |
| 6        | int16    | Setpoint — RW, Default 200                               |
| 7        | uint16   | Mode — RW, Default 1                                     |
| 10..14   | string   | "FLINT-DEMO" (5 Register, 10 ASCII-Zeichen)              |
| 20..21   | float32  | Energie (kWh) — monoton steigend, 1 kWh / Minute         |

### Input Registers (FC4, RO)

| Adresse  | Typ      | Inhalt                                  |
| -------- | -------- | --------------------------------------- |
| 0..1     | float32  | Live-Sensor — reiner 0.5 Hz Sinus, ±1.0 |
| 2        | uint16   | Drehzahl — Random Walk in [1200, 1800]  |

### Coils (FC1, RW)

Adressen 0..63 sind alle schreibbar, Default `false`. FC5 (Single) und FC15
(Multiple) funktionieren beide.

### Discrete Inputs (FC2, RO)

Adressen 0..15 zeigen ein **Lauflicht**: jede Sekunde wandert ein gesetztes Bit
um eine Stelle weiter. Praktisch um zu sehen, dass Polling tatsächlich frische
Daten holt.

## Verifizieren

In Flint:

1. Config-Node `Modbus Server` anlegen, `host=127.0.0.1`, `port=5502`,
   `defaultUnitId=1`.
2. `Modbus Read` auf den Canvas ziehen, Server auswählen.
3. FC3, Address 0, DataType float32, Polling 1 s — Debug-Node anhängen.

Im Debug-Panel muss eine Temperatur in der Größenordnung 21 °C ankommen, die
sich langsam ändert.

## Beispiel: Raw lesen + im Function Node zu float32 dekodieren

Statt den Codec im Read-Node erledigen zu lassen, kann man die rohen Register
holen und im Function Node mit der Buffer-API parsen. Das ist z.B. praktisch,
wenn ein Gerät mehrere Werte unterschiedlichen Typs in einem zusammenhängenden
Block liefert und man sie in einem Schritt auseinandernimmt.

**Modbus Read** — alle Defaults aus dem `Modbus Demo` Setup, nur:

```
Function Code: FC3 — Read Holding Registers
Address:       0
Quantity:      2
Data Type:     raw          ← roh, beide Repräsentationen werden ausgeliefert
Polling:       1000 ms
```

Bei FC3 / FC4 mit `dataType: raw` liefert der Read-Node **beide** Sichten in
derselben Message:

| Feld          | Format                  | Wofür                                      |
| ------------- | ----------------------- | ------------------------------------------ |
| `msg.payload` | `[]int` Wort-Array      | Direkt-Zugriff auf einzelne Register       |
| `msg.bytes`   | `[]int` Wire-Bytes      | `Buffer.from(msg.bytes)` für Byte-Parsing  |
| `msg.modbus`  | Metadaten (FC, Adresse) | Debugging / Round-Trip                     |

So wählt man im Function Node die für den Anwendungsfall passende Sicht ohne
Konvertierungs-Boilerplate.

**Function Node** — Float32 aus Wire-Bytes:

```javascript
// msg.bytes ist exakt das, was vom Bus kam (4 Bytes für 2 Register).
const buf = Buffer.from(msg.bytes);

msg.payload = buf.readFloatBE(0);   // ≈ 21.5 °C
msg.topic   = 'temperature';
return msg;
```

**Alternativ** — selbe Aufgabe über das Wort-Array (z.B. wenn man pro Register
unterschiedlich dekodieren will):

```javascript
// msg.payload = [reg0, reg1] als uint16 — Bytes selbst zusammenbauen.
const buf = Buffer.alloc(4);
buf.writeUInt16BE(msg.payload[0], 0);
buf.writeUInt16BE(msg.payload[1], 2);

msg.payload = buf.readFloatBE(0);
return msg;
```

**Fertiger Flow** liegt als `example-flow.json` neben dieser README — über
*Import* in der Editor-Toolbar einlesbar.

> **Tipp** für CDAB / BADC / DCBA Geräte: `buf.swap16()` bzw. `buf.swap32()`
> auf das Buffer-Objekt aufrufen, bevor `readFloatBE` läuft. Damit lassen sich
> alle vier Byte/Word-Order-Kombinationen im Function Node abdecken, ohne den
> Read-Node anzufassen.

## Function Codes

Unterstützt: **FC1, FC2, FC3, FC4, FC5, FC6, FC15, FC16**.

Nicht unterstützt (liefert `Illegal Function`, Code 0x01): FC7, FC11, FC12,
FC17, FC20–24, FC43.

## Exception Codes

Der Server liefert die Standard-Modbus-Exceptions:

| Code   | Bedeutung               | Wann                                        |
| ------ | ----------------------- | ------------------------------------------- |
| 0x01   | Illegal Function        | unbekannter / nicht unterstützter FC        |
| 0x02   | Illegal Data Address    | `addr + qty > 65536`                        |
| 0x03   | Illegal Data Value      | `qty` außerhalb der spec-erlaubten Range    |

## Eigenschaften / Grenzen

- Akzeptiert **jede** Unit-ID (Slave-ID) — im Demo-Kontext irrelevant
- Ein Mutex serialisiert alle Zugriffe auf den Datenstore (echte Devices
  serialisieren ohnehin pro Verbindung)
- Daten persistieren **nicht** über Neustarts — bei Stop ist der Setpoint
  wieder 200, Coils wieder alle `false`
- Adressraum-Grenzen sind die Modbus-Spec-Maxima: 2000 Coils/Discrete pro
  Read, 125 Register pro Read, 1968 Coils pro Write, 123 Register pro Write
