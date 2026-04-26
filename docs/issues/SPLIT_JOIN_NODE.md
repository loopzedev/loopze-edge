# Split & Join Nodes

## Beschreibung

Zwei komplementäre Nodes für **Sequenzverarbeitung** — der eine zerlegt eine Message in viele, der andere fügt viele Messages wieder zu einer zusammen. Sie sind das Standard-Pattern, um Listen/Streams/Batches in einem Flow zu verarbeiten:

```
[ Source ] → [ Split ] → [ N Verarbeitungs-Schritte ] → [ Join ] → [ Sink ]
```

Beide Nodes basieren auf einem neuen Message-Konzept **`msg.parts`**, das die Zugehörigkeit einer Message zu einer Sequenz beschreibt.

> Voraussetzung für das Konzept: Switch-/Filter-Operatoren wie `head`/`tail` (siehe `SWITCH_NODE.md` — dort bewusst weggelassen) können erst sinnvoll umgesetzt werden, wenn `msg.parts` existiert.

## Das `msg.parts`-Konzept

Wenn Split eine Message in N Teile zerlegt, hängt es an jede ausgehende Message ein `parts`-Objekt:

| Feld | Typ | Beschreibung |
|---|---|---|
| `id` | string | Gemeinsame Sequenz-ID (alle Messages der gleichen Zerlegung teilen diese ID) |
| `index` | number | 0-basierte Position in der Sequenz |
| `count` | number | Gesamtanzahl der Messages in der Sequenz (kann fehlen bei Streams) |
| `type` | string | `array`, `string`, `object`, `buffer` — wie wurde zerlegt |
| `ch` | string | Trennzeichen (nur bei `type=string`, für Rückbau) |
| `key` | string | Original-Key (nur bei `type=object`, für Rückbau) |
| `len` | number | Länge des Original-Datenfelds (für Buffer-Rückbau) |

`parts` reist mit der Message durch den Flow — zwischenliegende Nodes (Function, Change, ...) lassen es unberührt, sodass Join die Sequenz später rekonstruieren kann.

## Split Node

### Verhalten

- **1 Input**, **1 Output**
- Pro eingehender Message: zerlegt `msg.payload` (oder ein konfigurierbares Property) und sendet pro Element eine eigene Message
- Original-Message-Felder werden **kopiert**, nur `payload` (bzw. das gesplittete Feld) ersetzt
- `msg.parts` wird auf jeder Output-Message gesetzt
- Bestehendes `msg.parts` einer eingehenden Message wird in `msg.parts.parts` verschachtelt (für nested Splits) — siehe Node-RED-Verhalten

### Split-Modi (je nach Payload-Typ)

| Payload-Typ | Aufteilung | Konfiguration |
|---|---|---|
| **Array** | Pro Element eine Message | Optional: Chunks à N Elemente |
| **String** | An Trennzeichen aufteilen | Trennzeichen (Default: `\n`), optional Regex |
| **Object** | Pro Key eine Message, Key landet in `msg.parts.key` (oder konfigurierbar in einem Feld) | Key-Property-Name |
| **Buffer** | In Chunks à N Bytes oder an Byte-Sequenz | Chunk-Größe oder Trenn-Byte-Sequenz |

Bei nicht passendem Typ: Message wird unverändert weitergegeben + Warning geloggt.

### Konfiguration (Backend)

```json
{
  "property": "payload",
  "splt": "\n",
  "spltType": "str",
  "arraySplt": 1,
  "arraySpltType": "len",
  "stream": false,
  "addname": ""
}
```

| Feld | Beschreibung |
|---|---|
| `property` | Welches Property zerlegen (Default `payload`) |
| `splt` | Trennzeichen (String) oder Chunk-Größe (Buffer) |
| `spltType` | `str`, `bin` (Buffer-Pattern), `len` (Anzahl Bytes) |
| `arraySplt` | Bei Arrays: Größe der Sub-Arrays (1 = ein Element pro Message) |
| `arraySpltType` | `len` (fixe Größe) |
| `stream` | `true` = `msg.parts.count` wird nicht gesetzt (Stream-Modus), `false` = abgeschlossene Sequenz |
| `addname` | Bei Object-Split: in welches Property der Original-Key geschrieben wird (z.B. `topic`). Leer = nur in `msg.parts.key` |

### Beispiele

**Array zerlegen:**
```
Input:  msg.payload = [1, 2, 3]
Output: 3 Messages mit payload=1/2/3 und parts.{id, index, count=3, type:"array"}
```

**String zerlegen (Zeilen):**
```
Input:  msg.payload = "a\nb\nc"
Output: 3 Messages mit payload="a"/"b"/"c" und parts.{..., type:"string", ch:"\n"}
```

**Object zerlegen mit Key in topic:**
```
Input:  msg.payload = {a:1, b:2}
Config: addname = "topic"
Output: 2 Messages mit payload=1/2, topic="a"/"b", parts.{..., type:"object", key:"a"/"b"}
```

## Join Node

### Verhalten

- **1 Input**, **1 Output**
- Sammelt eingehende Messages, kombiniert sie zu einer einzigen Output-Message
- Hat **internen Zustand** (Per-Node-Buffer pro Sequenz-ID bzw. pro Topic)
- Sendet die kombinierte Message wenn ein **Trigger** auslöst (Count, Timeout, parts-Complete, ...)

### Modi

| Modus | Beschreibung |
|---|---|
| **automatic** | Nutzt `msg.parts` aus einem vorherigen Split. Sequenz-ID gruppiert, `count` triggert Send. Keine weitere Konfiguration nötig — exaktes Gegenstück zum Split. |
| **manual** | Ignoriert `msg.parts`. Nutzer konfiguriert Output-Typ und Trigger explizit. Auch für Messages, die nie durch einen Split liefen (z.B. Sensor-Aggregation pro Zeitfenster). |
| **reduce sequence** | Wendet eine Reduktions-Funktion auf die Sequenz an (z.B. Summe, Min/Max, Concat). Inspiriert von Node-RED, aber im MVP optional — siehe Offene Fragen. |

### Manual-Modus: Output-Typen

| Typ | Beschreibung |
|---|---|
| **string** | Mit Trennzeichen verbinden (z.B. `\n`) |
| **array** | Werte als Array zusammenfassen |
| **object** | Key/Value-Object — Key kommt aus konfigurierbarer Property (z.B. `msg.topic`) oder aus `msg.parts.key` |
| **buffer** | Mit optionaler Trenn-Byte-Sequenz verbinden |
| **merged object** | Wie object, aber Werte werden bei gleichem Key tief gemergt |

### Trigger (wann wird die kombinierte Message gesendet?)

| Trigger | Beschreibung |
|---|---|
| **automatic** | Sobald `count` aus `msg.parts` erreicht ist (nur im automatic-Modus) |
| **count N** | Nach genau N empfangenen Messages |
| **after timeout** | Nach X Sekunden Inaktivität (kein neues Message für die Sequenz) |
| **after specific message** | Wenn eine Message mit einem bestimmten Property-Wert eintrifft (z.B. `msg.complete = true`) |
| **manual reset** | Erst wenn eine Reset-Message kommt (z.B. mit `msg.reset = true`) |

Mehrere Trigger können kombiniert werden (whichever fires first).

### Per-Topic-Gruppierung (optional)

Wenn aktiv, hält Join **pro `msg.topic`** einen separaten Puffer und triggert unabhängig. Praktisch z.B. um Sensor-Werte pro Sensor-Topic zu aggregieren.

### Konfiguration (Backend)

```json
{
  "mode": "auto",
  "build": "string",
  "property": "payload",
  "propertyType": "msg",
  "key": "topic",
  "joiner": "\\n",
  "joinerType": "str",
  "accumulate": false,
  "timeout": 0,
  "count": 0,
  "reduceRight": false
}
```

| Feld | Beschreibung |
|---|---|
| `mode` | `auto`, `custom` (= manual), `reduce` |
| `build` | Output-Typ: `string`, `array`, `object`, `merged`, `buffer` (nur bei `custom`) |
| `property` | Welches Property aus jeder Message kombiniert wird (Default `payload`) |
| `propertyType` | `msg` (immer für Source) |
| `key` | Property-Name für Object-Keys (Default `topic`, fällt zurück auf `parts.key`) |
| `joiner` | Trennzeichen (String/Buffer) |
| `joinerType` | `str`, `bin` |
| `accumulate` | `true` = Sequenz wird nach jedem Send nicht geleert, sondern behalten (Sliding-Window-artig) |
| `timeout` | Timeout in Sekunden (0 = aus) |
| `count` | Trigger-Count (0 = aus) |
| `reduceRight` | Nur `reduce`-Modus: Reduktion von rechts |

### Beispiele

**Automatisches Join nach Split:**
```
Source → Split → Function (verarbeitet jedes Element) → Join (auto) → Sink
```
Join rekonstruiert das Original-Array/String/Object 1:1 aus `msg.parts`.

**Sensor-Aggregation pro Topic mit Timeout:**
```
mode: custom
build: array
key: topic
timeout: 5
```
→ Pro Topic werden die Messages 5s lang gesammelt, dann als Array gesendet.

**Bis Sentinel-Message:**
```
mode: custom
build: array
trigger: after specific message → msg.eof === true
```

## Implementierung

### Backend

#### `internal/nodes/split.go`
- `flow.NodeInstance`, **Inputs:** 1, **Outputs:** 1
- `OnMessage`:
  1. Property-Wert holen
  2. Typ erkennen (Array/String/Object/Buffer)
  3. In Teile zerlegen, pro Teil neue Message via `msg.Clone()` + Property setzen
  4. `parts`-Feld setzen (verschachtelt falls vorher schon vorhanden)
  5. Sequenz-ID via `generateID()` (gleiche Funktion wie Message-IDs)
  6. Sequenziell senden

#### `internal/nodes/join.go`
- `flow.NodeInstance`, **Inputs:** 1, **Outputs:** 1
- Hält `map[string]*sequenceBuffer` (Key = Sequenz-ID oder Topic)
- Per Sequenz: Messages sammeln, Trigger prüfen
- Bei Trigger: Kombinieren, senden, Buffer leeren (außer `accumulate=true`)
- Timeout: `time.AfterFunc` pro Sequenz, beim Trigger canceln
- Thread-Safety: `sync.Mutex` um die Buffer-Map

#### Erweiterung von `flow.Message`
Aktuell ist `Message.data` ein flaches `map[string]any` — `parts` kann darin als regulärer Key liegen. Kein API-Change nötig, nur Konvention dokumentieren:
- `msg.Get("parts.id")`, `msg.Get("parts.index")`, ... funktioniert dank Dot-Path bereits
- Helper-Funktion in `flow` package vorschlagen: `msg.Parts() *Parts` für typsicheren Zugriff

Bei `Clone()` darauf achten, dass `parts` mitkopiert wird (passiert automatisch, weil Teil von `data`).

### Frontend

#### `SplitConfig.vue`
- Property-Wahl (`MsgFieldEditor`)
- Auto-Erkennung des Payload-Typs in der UI mit Hinweistext
- Felder dynamisch je nach erwartetem Typ:
  - String → Trennzeichen-Input + Regex-Toggle
  - Array → Chunk-Size-Input
  - Object → "Key in Property" Input
  - Buffer → Chunk-Size oder Byte-Pattern
- Stream-Toggle (Checkbox)

#### `JoinConfig.vue`
- Modus-Tabs (Auto / Manual / Reduce)
- Im Manual-Modus: Output-Typ-Dropdown, je nach Typ unterschiedliche Felder
- Trigger-Sektion: Count, Timeout, "complete on property", Reset
- Per-Topic-Gruppierung als Checkbox

#### Node-Komponenten
- BaseNode mit Category `function` (oder neue Kategorie `sequence`)
- Split-Body: zeigt Trennzeichen/Chunk-Größe kompakt
- Join-Body: zeigt Modus + Trigger kompakt

### Node-Registrierung

```go
registry.Register("split", nodes.NewSplitNode, nodes.SplitTypeInfo())
registry.Register("join",  nodes.NewJoinNode,  nodes.JoinTypeInfo())
```

## Zusammenspiel mit anderen Nodes

- **Switch Node**: Sobald `msg.parts` existiert, können dort Operatoren wie `head N`, `tail N`, `index between` ergänzt werden (siehe `SWITCH_NODE.md` Offene Fragen).
- **Function Node**: Kann `parts` bewusst manipulieren (z.B. eigene Sequenzen synthetisieren) — keine Sonderbehandlung nötig.
- **Change Node**: Kann `parts` löschen, falls eine Sequenz absichtlich "abgeschnitten" werden soll.
- **Debug Node**: Sollte `parts` im Tree-View sichtbar machen (passiert automatisch, da reguläres Property).

## Abhängigkeiten

- `flow.Message.Clone()` — vorhanden
- `flow.Message.Get/Set` mit Dot-Path — vorhanden
- `generateID()` für Sequenz-IDs — vorhanden
- Per-Node-State im Join: Engine erlaubt das bereits (Function Node hat State)
- Frontend: `MsgFieldEditor`, `FormSelect` aus `components/config/`

## Offene Fragen

1. **`reduce sequence`-Modus** im Join — MVP oder später? Erfordert eingebettete Expression-Engine (JSONata in Node-RED). Vorschlag: **später**, MVP nur `auto` + `custom`.
2. **Verschachtelte Splits** — soll `parts.parts`-Verschachtelung explizit unterstützt werden, oder beim ersten MVP nur eine Ebene?
3. **Buffer-Support** — wie wichtig? MQTT-Payloads kommen oft als Buffer rein. Vorschlag: **MVP ja**, da geringer Mehraufwand.
4. **Per-Topic-Gruppierung** — eigenständiger Modus oder Option in `custom`-Modus? Aktuell als Option modelliert.
5. **Helper `msg.Parts()`** in `flow`-Package — typsicher gut, aber bricht das "alles ist gleich"-Prinzip der flachen Map. Alternative: nur Konvention + Konstanten für Key-Namen.
6. **Memory-Schutz im Join**: Was passiert bei nie auslösenden Triggern (Sequenz-ID kommt nie auf `count`)? Vorschlag: konfigurierbares Max-Buffer-Alter (Default: 10min) → verworfen + Warning.
