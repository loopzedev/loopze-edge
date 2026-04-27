# Issue: Template Node — Texte aus Vorlagen zusammenbauen

## Status: Proposed

## Problembeschreibung

Aktuell gibt es keinen einfachen Weg, mehrzeilige Texte mit Variablen aus
`msg`, `flow.` oder `global.` zu erzeugen. Wer z.B. eine MQTT-Payload, eine
Benachrichtigung oder ein JSON-Objekt aus mehreren Quellen zusammenbauen will,
muss heute auf den Function Node ausweichen — Code für eine Aufgabe, die
deklarativ schöner ist.

In Node-RED erledigt das der **Template Node**: ein Textfeld mit
Mustache-Platzhaltern, das Ergebnis landet als String in `msg.payload`
(oder einem anderen Property).

## Sichtweise / Begründung

- **Bediener-UX**: Eine Textarea mit `{{payload}}`-Platzhaltern ist für
  Nicht-Programmierer deutlich zugänglicher als JS-Code im Function Node.
- **Niedrige Komplexität**: Mustache als Template-Sprache ist klein, etabliert,
  hat Go-Libs (`github.com/cbroglie/mustache` o.ä.).
- **Pattern bereits da**: Scope-Auswahl (`msg`/`flow`/`global`) inkl.
  Storage-Schalter (`memory`/`persistent`) existiert beim Change Node — Template
  Node nutzt dasselbe Mental-Model.
- **Häufige Use Cases**:
  - Notification-Texte: `"Sensor {{topic}} meldet {{payload}}°C"`
  - JSON-Bodies für HTTP/MQTT: `{"id": "{{flow.deviceId}}", "v": {{payload}}}`
  - Log-Zeilen, Statusmeldungen, Dashboard-Texte

## Anforderungen

### 1. Template-Quelle

Eine **Textarea** im Properties-Panel mit Mustache-Syntax.

| Platzhalter | Bedeutung |
|---|---|
| `{{payload}}` | `msg.payload` |
| `{{topic}}` | `msg.topic` |
| `{{<dot.path>}}` | beliebiges `msg.<dot.path>` |
| `{{flow.<key>}}` | Wert aus Flow-Context |
| `{{global.<key>}}` | Wert aus Global-Context |
| `{{{value}}}` | unescaped (bei Output-Format `html` relevant) |

Mustache-Sections (`{{#list}}…{{/list}}`, `{{^missing}}…{{/missing}}`) werden
unterstützt — Standard-Mustache-Verhalten, keine Sondersemantik.

### 2. Output-Ziel

| Feld | Beschreibung | Default |
|---|---|---|
| `field` | Property-Name am Output (Dot-Path) | `payload` |
| `fieldType` | Scope: `msg`, `flow`, `global` | `msg` |

Bei `flow`/`global` erscheint **hinter dem Property-Feld** ein
Storage-Dropdown (`memory` / `persistent`) — analog Change Node.

### 3. Output-Format

Dropdown bestimmt die Nachbearbeitung des gerenderten Strings:

| Wert | Verhalten |
|---|---|
| `plain` | String wird unverändert als String ausgegeben (Default) |
| `json` | Ergebnis wird mit `json.Unmarshal` geparst — bei Fehler: Catchable Error |
| `yaml` | Ergebnis wird mit YAML-Parser geparst — bei Fehler: Catchable Error |

Phase 1 implementiert nur `plain` und `json`. `yaml` ist nice-to-have, kann
nachgezogen werden, sobald Bedarf besteht.

### 4. Syntax-Modus

Dropdown:

| Wert | Beschreibung |
|---|---|
| `mustache` | Template wird gerendert (Default) |
| `plain` | Template wird 1:1 durchgereicht — nützlich für statische Texte mit `{{`/`}}` |

### 5. Fehlerbehandlung

- Ungültige Mustache-Syntax → Node geht auf `red` / `"template error"`,
  Message wird als Catchable Error dem Catch Node zugeführt.
- Unbekannter Platzhalter → leer (Mustache-Standard), kein Fehler.
- JSON-/YAML-Parse-Fehler bei entsprechendem Output-Format → Catchable Error.

### 6. Status

- Idle: kein Status (wie Change Node).
- Bei Render-Fehler: `red` / `"template error"`.

## Technische Skizze

### Backend — `internal/nodes/template.go` (neu)

```go
type TemplateNode struct {
    config flow.NodeConfig
    send   flow.SendFunc
    status flow.StatusFunc

    template     string                 // raw template
    field        string                 // dot-path
    fieldType    string                 // "msg" | "flow" | "global"
    fieldStorage string                 // "memory" | "persistent"
    format       string                 // "plain" | "json" | "yaml"
    syntax       string                 // "mustache" | "plain"

    contextProvider flow.ContextProvider
    parsed          *mustache.Template   // pre-parsed in Init
}
```

- **Inputs:** 1, **Outputs:** 1
- Implementiert `ContextProvider`-Konsum analog Function/Change Node
- Pre-Parse des Templates in `Init` (Fail-fast bei Syntaxfehler beim Deploy)
- Bei `syntax == "plain"`: kein Parsing, Template-String direkt als Output
- In `HandleMessage`:
  - Build View-Map: `{payload, topic, ...msg-fields, flow: lookup(...), global: lookup(...)}`
  - Render → String
  - Format-Postprocess (`json.Unmarshal` falls `format == "json"`)
  - Schreibe Ergebnis nach `field` im jeweiligen Scope
  - `send(0, msg)`

### Mustache-Lib

`github.com/cbroglie/mustache` — kleine, abhängigkeitsfreie Go-Implementation,
unterstützt Sections und Lambdas-light. Einbindung über `go.mod`.

### Node-Registrierung — `internal/server/server.go`

```go
registry.Register("template", nodes.NewTemplateNode, nodes.TemplateTypeInfo())
```

```go
func TemplateTypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:        "template",
        Category:    "function",
        Label:       "Template",
        Description: "Build a string from a Mustache template using msg/flow/global values",
        Icon:        "mdi-text-box-outline",
        Defaults: map[string]any{
            "template":      "This is the payload: {{payload}}!",
            "field":         "payload",
            "fieldType":     "msg",
            "fieldStorage":  "memory",
            "format":        "plain",
            "syntax":        "mustache",
        },
        Inputs:  1,
        Outputs: 1,
    }
}
```

### Frontend — `frontend/src/components/nodes/TemplateNode.vue` (neu)

- BaseNode mit Kategorie `function`
- Body zeigt z.B. die ersten ~24 Zeichen des Templates: `"Sensor {{topic}} ..."`

### Frontend — `frontend/src/components/config/TemplateConfig.vue` (neu)

- Mehrzeilige Textarea (monospace) für das Template
- Property-Feld (Dot-Path) + Scope-Dropdown (`msg`/`flow`/`global`)
- Storage-Dropdown bei `flow`/`global` (DRY: gleiche Komponente wie Change/Function)
- Output-Format-Dropdown (`plain`/`json`)
- Syntax-Dropdown (`mustache`/`plain`)
- Alle Felder via `useNodeProperty`

### Frontend — Verdrahtung

- `frontend/src/views/FlowEditor.vue`: `<template #node-template>` + Import
- `frontend/src/components/PropertyPanel.vue`: `<TemplateConfig>` für `type === 'template'`
- `frontend/src/components/NodeIcon.vue`: Icon-Eintrag für `template`

## Betroffene Dateien

### Backend
- `go.mod` — Dependency `github.com/cbroglie/mustache`
- `internal/nodes/template.go` (neu)
- `internal/nodes/template_test.go` (neu) — Pre-Parse-Fehler, Render mit
  msg/flow/global, Output-Formate, Syntax-Modus `plain`
- `internal/server/server.go` — Registrierung

### Frontend
- `frontend/src/components/nodes/TemplateNode.vue` (neu)
- `frontend/src/components/config/TemplateConfig.vue` (neu)
- `frontend/src/views/FlowEditor.vue` — Slot + Import
- `frontend/src/components/PropertyPanel.vue` — Config-Mapping
- `frontend/src/components/NodeIcon.vue` — Icon-Eintrag

## Abhängigkeiten

- `flow.ContextProvider` (existiert)
- `flow.ContextStore` mit Storage-Auswahl (existiert, vom Change Node genutzt)
- Catch Node für Fehlerweiterleitung (existiert)

## Out of Scope für Phase 1

- **YAML-Output** — wird nachgezogen, sobald nötig
- **Custom Delimiters** (`{{=<% %>=}}`) — Mustache kann das, im UI aber selten
  gewünscht; wenn Bedarf entsteht, als optionales Feld nachreichbar
- **Partials/Includes** über mehrere Nodes — kein Use Case in Sicht
- **Live-Preview im Properties-Panel** — nett, aber kein MVP

## Offene Fragen

- Soll der Output-Wert bei `format: json` und Parse-Fehler stattdessen den
  rohen String belassen (statt Catchable Error)? — Vorschlag: nein, klarer
  Fehler ist konsistenter mit den anderen Nodes.
