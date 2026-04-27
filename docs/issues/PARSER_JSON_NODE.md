# Issue: JSON Parser Node — Payload zwischen JSON-String und Struktur konvertieren

## Status: Proposed

## Kontext

Erster konkreter Parser aus dem Konzept in `PARSER_NODES.md`. Wer
MQTT-Topics, HTTP-APIs oder NATS-Subjects mit JSON-Payloads anbindet,
braucht heute einen Function Node mit `JSON.parse` / `JSON.stringify` —
ein klarer Fall für eine deklarative Lösung.

## Problembeschreibung

`msg.payload` kommt aus MQTT-In oder HTTP-In typischerweise als
**String** oder **`[]byte` (Buffer)** an. Damit Switch/Change/Template
auf einzelne Felder zugreifen können, muss der Payload zu einem
strukturierten Wert (`map`, `[]any`, Skalar) geparst werden. Beim
Versenden in die andere Richtung muss er wieder zu einem String
serialisiert werden.

Der **JSON Parser Node** macht genau das — bidirektional, mit
Fehlerbehandlung und ohne Code.

## Sichtweise / Begründung

- **Bediener-UX**: zwei Klicks (Drop-In, Property prüfen) statt
  Function-Node + JS-Code.
- **Häufigste Aufgabe in jedem Flow**: jede MQTT-Verbindung mit
  strukturiertem Payload braucht das.
- **Konsistent mit Template Node** — der hat bereits `format: json`
  als Output-Postprocessing; der JSON-Parser ist das Pendant für die
  Eingangsseite.

## Anforderungen

### 1. Property

| Feld | Beschreibung | Default |
|---|---|---|
| `property` | Property am `msg`-Objekt (Dot-Path) | `payload` |

Phase 1: nur `msg.<property>`. Scope-Auswahl (`flow`/`global`) bewusst
weggelassen — siehe `PARSER_NODES.md`.

### 2. Action

Dropdown bestimmt die Konvertierungs-Richtung:

| Wert | Verhalten |
|---|---|
| `auto` | **Default.** Wenn Wert `string` oder `[]byte` → parse zu Struktur. Sonst → stringify zu String. |
| `parse` | Erzwingt Parsen. Fehler, wenn Wert kein String/Buffer ist. |
| `stringify` | Erzwingt Serialisieren. Fehler, wenn Wert bereits ein String ist. |

`auto` ist bewusst der Default: in der Praxis ist die Richtung durch
den vorhergehenden Node klar (MQTT-In → parse, MQTT-Out davor →
stringify). Wer Strenge braucht, schaltet auf `parse` / `stringify`.

### 3. Pretty-Print (nur bei stringify)

| Feld | Beschreibung | Default |
|---|---|---|
| `indent` | Anzahl Spaces für `json.MarshalIndent`. `0` = kompakt (kein Indent). | `0` |

UI: Number-Input (0–8), nur sichtbar bei Action `stringify` oder `auto`.

### 4. Status / Fehlerbehandlung

- Idle: kein Status.
- Parse-Fehler (`json.Unmarshal` schlägt fehl) →
  Status `red` / `"json parse error"`, Catchable Error.
- Wert hat falschen Typ für die gewählte Action →
  Status `red` / `"json type error"`, Catchable Error.
- Erfolgreich: Status bleibt unverändert (kein „green" für jede
  Message — vermeidet Status-Geflacker bei hoher Frequenz, analog
  Change/Template).

### 5. Inputs / Outputs

- **1 Input**, **1 Output**.
- Property wird **am selben Property-Pfad** überschrieben — also
  `msg.payload` rein, `msg.payload` raus (in der jeweils anderen Form).

## Beispiele

### Beispiel 1 — MQTT-JSON parsen

```
[MQTT-In: sensor/temp]  →  [JSON: action=auto]  →  [Switch: msg.payload.value > 30]
```

Eingang: `msg.payload = '{"value": 25.4, "unit": "C"}'` (String)
Ausgang: `msg.payload = {value: 25.4, unit: "C"}` (Object)

### Beispiel 2 — HTTP-Body serialisieren

```
[Function: msg.payload = {ok:true}]  →  [JSON: action=stringify, indent=2]  →  [HTTP-Out]
```

Eingang: `msg.payload = {ok: true}` (Object)
Ausgang: `msg.payload = "{\n  \"ok\": true\n}"` (String, Pretty)

### Beispiel 3 — Buffer aus Modbus parsen

```
[Modbus-In: 0/0..63]  →  [JSON: action=parse]  →  [Debug]
```

Eingang: `msg.payload = []byte('{"reg0": 1234}')`
Ausgang: `msg.payload = {reg0: 1234}`

## Technische Skizze

### Backend — `internal/nodes/parser_json.go` (neu)

```go
type JSONParserNode struct {
    config flow.NodeConfig
    send   flow.SendFunc
    status flow.StatusFunc

    property string  // dot-path, default "payload"
    action   string  // "auto" | "parse" | "stringify"
    indent   int     // 0..8
}
```

- **Inputs:** 1, **Outputs:** 1
- Implementiert `flow.NodeInstance`.
- Kein `ContextProvider` nötig (kein flow/global Scope in Phase 1).
- In `HandleMessage`:
  1. Wert via `msg.Get(n.property)` lesen.
  2. Action-Logik (siehe unten).
  3. Ergebnis via `msg.Set(n.property, …)` zurückschreiben.
  4. `send(0, msg)`.

### Action-Logik (Pseudocode)

```go
func (n *JSONParserNode) convert(value any) (any, error) {
    switch n.action {
    case "parse":
        return parseAny(value)
    case "stringify":
        return stringify(value, n.indent)
    case "auto":
        if isStringish(value) {
            return parseAny(value)
        }
        return stringify(value, n.indent)
    }
}

func parseAny(v any) (any, error) {
    var data []byte
    switch x := v.(type) {
    case string:
        data = []byte(x)
    case []byte:
        data = x
    default:
        return nil, fmt.Errorf("json parse: expected string or []byte, got %T", v)
    }
    var parsed any
    if err := json.Unmarshal(data, &parsed); err != nil {
        return nil, err
    }
    return parsed, nil
}

func stringify(v any, indent int) (string, error) {
    if _, ok := v.(string); ok {
        return "", fmt.Errorf("json stringify: value already a string")
    }
    if indent > 0 {
        b, err := json.MarshalIndent(v, "", strings.Repeat(" ", indent))
        return string(b), err
    }
    b, err := json.Marshal(v)
    return string(b), err
}
```

### Node-Registrierung — `internal/server/server.go`

```go
registry.Register("json", nodes.NewJSONParserNode, nodes.JSONParserTypeInfo())
```

```go
func JSONParserTypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:        "json",
        Category:    "parser",
        Label:       "JSON",
        Description: "Convert JSON strings to objects and back",
        Icon:        "json",
        Defaults: map[string]any{
            "property": "payload",
            "action":   "auto",
            "indent":   0,
        },
        Inputs:  1,
        Outputs: 1,
    }
}
```

> **Offen — Kategorie:** Neue Palette-Kategorie `parser` einführen,
> oder bei `function` mitlaufen lassen? Vorschlag: neue Kategorie,
> damit XML/CSV später visuell zusammen sitzen. Abstimmen mit
> `PROPERTIES_PANEL.md` / Palette-Konvention.

### Frontend — `frontend/src/components/nodes/JSONParserNode.vue` (neu)

- BaseNode mit Kategorie `parser` (oder `function`, siehe oben).
- Body zeigt z.B. `payload · auto` oder `payload → string`.

### Frontend — `frontend/src/components/config/JSONParserConfig.vue` (neu)

- Property-Input (Dot-Path).
- Action-Dropdown (`auto` / `parse` / `stringify`).
- Indent-Number-Input (0–8), nur sichtbar bei `stringify` oder `auto`.
- Alle Felder via `useNodeProperty`.

### Frontend — Verdrahtung

- `frontend/src/views/FlowEditor.vue`: `<template #node-json>` + Import.
- `frontend/src/components/PropertyPanel.vue`: `<JSONParserConfig>` für `type === 'json'`.
- `frontend/src/components/NodeIcon.vue`: Icon-Eintrag für `json`.

## Betroffene Dateien

### Backend
- `internal/nodes/parser_json.go` (neu)
- `internal/nodes/parser_json_test.go` (neu) — auto/parse/stringify
  Branches, String und `[]byte` Eingaben, Pretty-Print, Fehlerfälle
  (ungültiges JSON, falscher Typ).
- `internal/server/server.go` — Registrierung.

### Frontend
- `frontend/src/components/nodes/JSONParserNode.vue` (neu)
- `frontend/src/components/config/JSONParserConfig.vue` (neu)
- `frontend/src/views/FlowEditor.vue` — Slot + Import.
- `frontend/src/components/PropertyPanel.vue` — Config-Mapping.
- `frontend/src/components/NodeIcon.vue` — Icon-Eintrag.

## Abhängigkeiten

- `flow.Message` mit `Get`/`Set` (existiert)
- Catch Node für Fehlerweiterleitung (existiert)
- Standard-Library: `encoding/json` (kein neues Modul)

## Out of Scope für Phase 1

- **flow./global. Scopes** — siehe `PARSER_NODES.md`.
- **JSON-Schema-Validierung** — eigener Node später.
- **JSONPath / Sub-Pfad-Extraktion** — Change Node deckt das nach
  dem Parsen ab.
- **Streaming für sehr große Payloads** — kein Use Case.

## Offene Fragen

- **Palette-Kategorie**: neue Gruppe `parser` oder bei `function`
  einsortieren? (siehe oben)
- **Type-Name**: `json` (kurz, klar) oder `parser-json` (explizit
  Namespace)? Vorschlag `json` — analog zu Node-RED, kürzer.
- **Indent-Range**: hartes Cap bei 8 oder weiter offen lassen?
  Vorschlag: 0–8 reicht praktisch.
