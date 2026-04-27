# Issue: Parser Nodes — Payload-Konvertierung zwischen Formaten

## Status: Proposed

## Problembeschreibung

Flint-Flows tauschen Daten mit der Außenwelt aus: MQTT-Topics liefern
Sensorwerte als JSON-Strings, HTTP-APIs antworten mit XML, Industrie-Tools
exportieren CSV, Modbus liefert rohe Bytes. Damit nachgelagerte Nodes
(Switch, Change, Function, Template …) sinnvoll arbeiten können, muss die
Payload in ein **strukturiertes Format** überführt werden — und vor dem
Senden wieder in den Wire-Form-String gepackt werden.

Heute landet das im Function Node mit `JSON.parse(...)` /
`JSON.stringify(...)`. Funktioniert, aber:

- Bediener-UX leidet: Code für eine 80%-Standardaufgabe.
- XML/CSV gibt's in Goja gar nicht out-of-the-box.
- Fehlerbehandlung muss jedes Mal manuell verdrahtet werden.
- Kein einheitliches Verhalten zwischen Flows / Nodes.

Lösung: drei dedizierte **Parser Nodes**, deklarativ konfigurierbar,
mit einheitlicher API und Status/Catch-Anbindung.

## Sichtweise / Begründung

- **Bediener-UX vor Designer-Power**: Ein Dropdown „String → Object" ist
  zugänglicher als JS-Code. Die häufigsten Use Cases (MQTT-JSON-Payload
  parsen) decken wir mit zwei Klicks ab.
- **Ein gemeinsames Mental-Model** für alle drei Formate: gleiche Felder
  (Property, Action), gleiche Fehlerwege, gleiches Status-Verhalten.
- **Pendant zum Template Node**: Template baut Strings *aus* Daten —
  Parser-Nodes ziehen Daten *aus* Strings. Beide sitzen im selben
  "Function"-Bereich der Palette.

## Geplante Nodes

| Node | Eingabe (Wire-Form) | Ausgabe (parsed) | Phase |
|---|---|---|---|
| **JSON** | `string` / `[]byte` (Buffer) | `map` / `[]any` / Skalar | **Phase 1 — siehe `PARSER_JSON_NODE.md`** |
| **CSV** | `string` / `[]byte` | `[]map[string]any` (mit Header) oder `[][]any` | Phase 2 |
| **XML** | `string` / `[]byte` | `map[string]any` (Element-Tree) | Phase 3 |

Alle Parser können in beide Richtungen arbeiten (String/Buffer ⇄
strukturiert), gesteuert über ein Action-Dropdown:

| Action | Verhalten |
|---|---|
| `auto` | Heuristik: String/Buffer → parse; alles andere → stringify |
| `parse` | Erzwingt Parsen — Fehler bei nicht-String/Buffer |
| `stringify` | Erzwingt Serialisieren — Fehler bei String/Buffer |

`auto` ist der Default und deckt 90% der Fälle ab.

## Gemeinsame Anforderungen

### 1. Property-Auswahl

| Feld | Beschreibung | Default |
|---|---|---|
| `property` | Property am `msg`-Objekt (Dot-Path) | `payload` |

In Phase 1 wird **nur `msg.<property>`** unterstützt — kein flow/global
Scope. Begründung: Parser laufen typischerweise direkt nach einem
Eingangs-Node (MQTT-In, HTTP-In) und schreiben ins selbe Property
zurück. Scope-Auswahl kann nachgezogen werden, sobald ein konkreter
Use Case sie braucht.

### 2. Buffer-Kompatibilität

Eingabe darf ein **Buffer** sein (`[]byte` aus dem Go-Backend, künftig
auch das `Buffer`-Objekt aus `BUFFER_API.md`). Parser konvertiert
intern via `string(b)` bzw. `[]byte(s)` — UTF-8 wird vorausgesetzt.

### 3. Status / Fehlerbehandlung

- Idle: kein Status-Text (analog Change/Template).
- Bei Parse-/Stringify-Fehler: Status `red` / `"<format> parse error"`,
  Message wird als **Catchable Error** an Catch Nodes geleitet
  (gleiches Pattern wie Template Node bei `format: json`-Fehler).
- Wenn Action `parse` ist und das Property kein String/Buffer ist →
  Catchable Error.
- Wenn Action `stringify` ist und das Property bereits ein String ist
  → Catchable Error.

### 4. Inputs / Outputs

- **1 Input**, **1 Output** — Parser routen nicht, sie konvertieren nur.

## Out of Scope für Phase 1

- **Schema-Validierung** (JSON-Schema, XSD) — separate Nodes, später.
- **Streaming-Parser** für sehr große Payloads — bisher kein Use Case.
- **Custom-Encodings** außer UTF-8.
- **flow./global. Scopes** — siehe oben.

## Reihenfolge / Roadmap

1. **JSON Parser** (Phase 1) — siehe `PARSER_JSON_NODE.md`. Höchste
   Priorität, weil jeder MQTT/HTTP-Flow ihn braucht.
2. **CSV Parser** (Phase 2) — zweithäufigster Use Case (Industrie-
   Exporte, Reports). Eigenes Issue, sobald JSON steht.
3. **XML Parser** (Phase 3) — seltener, aber unverzichtbar für
   SOAP-/Legacy-APIs. Eigenes Issue, sobald CSV steht.

Jeder Parser bekommt sein eigenes Issue mit detaillierter Spezifikation.
Dieses Dokument bleibt als gemeinsames Konzept-Papier.

## Abhängigkeiten

- `flow.Message` mit `Get`/`Set` (existiert)
- Catch Node für Fehlerweiterleitung (existiert)
- `BUFFER_API.md` — wenn vorhanden, kann der Parser den `Buffer`-Typ
  direkt akzeptieren; ohne ihn arbeiten wir auf rohen `[]byte`-Slices
