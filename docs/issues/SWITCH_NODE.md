# Switch Node

## Beschreibung

Der Switch Node leitet eingehende Messages anhand konfigurierbarer Bedingungen auf einen oder mehrere Ausgänge weiter. Er ist das zentrale Werkzeug für **Routing/Verzweigung** in einem Flow — analog zu einem `if/elseif/else` oder `switch`-Statement, aber ohne Code.

Pendant zum Change Node: Während Change Daten **manipuliert**, **routet** Switch nur — die Message wird unverändert weitergegeben, lediglich der Ausgang wird gewählt.

## Verhalten

- **1 Input**, **N Outputs** (N = Anzahl der Regeln)
- Jede Regel entspricht **einem Output-Port** (Reihenfolge = Port-Reihenfolge von oben nach unten)
- Pro eingehender Message werden die Regeln sequenziell ausgewertet
- Die Message wird **unverändert** an alle treffenden Outputs gesendet (kein Klon nötig solange der Empfänger sie nicht mutiert — Implementierung muss aber ggf. defensiv klonen, siehe Implementierung)

### Auswertungs-Modi

| Modus | Beschreibung |
|---|---|
| **stop after first match** (Default) | Sobald eine Regel matched, werden weitere Regeln übersprungen. Klassisches `if/elseif`-Verhalten. |
| **check all rules** | Alle Regeln werden ausgewertet, jede passende Regel sendet auf ihren Output. Eine Message kann so an mehreren Outputs erscheinen. |

## Vergleichswert / Property

Wie beim Change Node wird der **zu prüfende Wert** über Scope + Property gewählt:

| Scope | Beispiel |
|---|---|
| **msg.** | `msg.payload`, `msg.topic`, `msg.foo.bar` |
| **flow.** | Wert aus dem Flow-Context |
| **global.** | Wert aus dem Global-Context |

Bei `flow.`/`global.` erscheint zusätzlich das **Storage-Dropdown** (`memory` / `persistent`) — identisch zum Change Node.

## Operatoren (pro Regel)

### Wert-Vergleiche

| Operator | Symbol | Beschreibung |
|---|---|---|
| **==** | `==` | Gleichheit (lose, mit Typkonvertierung) |
| **!=** | `!=` | Ungleichheit |
| **<** | `<` | Kleiner als |
| **<=** | `<=` | Kleiner oder gleich |
| **>** | `>` | Größer als |
| **>=** | `>=` | Größer oder gleich |
| **is between** | `[a..b]` | Wert liegt im Intervall [a, b] (zwei Wert-Felder) |
| **contains** | `⊃` | String/Array enthält Wert |
| **matches regex** | `.*` | Regex-Match auf String-Property |

### Typ-Prüfungen (kein Wert nötig)

| Operator | Beschreibung |
|---|---|
| **is true** | Wert ist `true` |
| **is false** | Wert ist `false` |
| **is null** | Wert ist `null` oder fehlt komplett |
| **is not null** | Wert existiert und ist nicht null |
| **is empty** | String/Array/Object ist leer |
| **is not empty** | Gegenstück zu `is empty` |
| **is of type** | Vergleich gegen Typ-Dropdown: `string`, `number`, `boolean`, `array`, `object`, `buffer`, `null`, `undefined` |

### Default

| Operator | Beschreibung |
|---|---|
| **otherwise** | Catch-all. Matched genau dann, wenn vorher **keine** Regel im `stop after first match`-Modus zugetroffen hat. Sollte als letzte Regel platziert werden. |

## Wert-Typen (rechts vom Operator)

Identisch zum Change Node — die Vergleichswerte können statisch oder aus anderen Quellen kommen:

| Typ | Beschreibung |
|---|---|
| **msg.** | Wert aus einem anderen Message-Property |
| **flow.** | Wert aus Flow-Context (mit Storage-Dropdown) |
| **global.** | Wert aus Global-Context (mit Storage-Dropdown) |
| **string** | Statischer String |
| **number** | Statische Zahl |
| **boolean** | `true` / `false` |
| **JSON** | Geparstes JSON-Objekt/Array |
| **Umgebungsvariable** | Wert aus ENV |
| **previous value** | Wert aus letzter Auswertung dieser Property (nur bei sinnvollen Operatoren — z.B. `!=` für "hat sich geändert") |

## UI-Layout

```
Property:    ▼ msg. [payload                        ]

Regeln:
  ≡  ▼ ==           ▼ string  [active        ]   ✕      → Output 1
  ≡  ▼ >            ▼ number  [10            ]   ✕      → Output 2
  ≡  ▼ matches re   ▼ string  [^err_         ]   ✕      → Output 3
  ≡  ▼ otherwise                                  ✕      → Output 4
       [ + Regel hinzufügen ]

Modus:  ◉ stop after first match   ○ check all rules
```

- Sortierbare Liste (Drag-Handle `≡`) — Reihenfolge bestimmt **Port-Reihenfolge**
- Pro Regel: Operator-Dropdown, Wert-Typ-Dropdown, Wert-Input (zwei Inputs bei `is between`/`index between`), Löschen-Button
- Beim Hinzufügen/Entfernen einer Regel wird ein Output-Port hinzugefügt/entfernt — bestehende Wires bleiben an ihrer Regel hängen (Wire-Map über Regel-ID, nicht Port-Index)

> Siehe auch `NODE_AND_MULTIOUTPUT.md` — die dort beschriebenen Probleme mit Multi-Output-Skalierung müssen für den Switch Node sauber gelöst sein, sonst wird die UI unbenutzbar.

## Konfiguration (Backend)

```json
{
  "property": "payload",
  "propertyType": "msg",
  "propertyStorage": "memory",
  "checkall": false,
  "rules": [
    { "id": "r1", "t": "eq",      "v": "active",  "vt": "str" },
    { "id": "r2", "t": "gt",      "v": "10",       "vt": "num" },
    { "id": "r3", "t": "regex",   "v": "^err_",   "vt": "str", "case": false },
    { "id": "r4", "t": "btwn",    "v": "0",  "vt": "num", "v2": "100", "v2t": "num" },
    { "id": "r5", "t": "else" }
  ]
}
```

### Top-Level-Felder

| Feld | Beschreibung |
|---|---|
| `property` | Name der zu prüfenden Property (ohne Scope-Prefix) |
| `propertyType` | Scope: `msg`, `flow`, `global` |
| `propertyStorage` | `memory` / `persistent` (nur bei flow/global) |
| `checkall` | `false` = stop after first match (Default), `true` = alle Regeln prüfen |

### Regel-Felder

| Feld | Beschreibung |
|---|---|
| `id` | Stabile ID (für Wire-Mapping über Reihenfolge-Änderungen hinweg) |
| `t` | Operator (siehe Operator-Tabelle, Kürzel s.u.) |
| `v` | Vergleichswert |
| `vt` | Wert-Typ: `msg`, `flow`, `global`, `str`, `num`, `bool`, `json`, `env`, `prev` |
| `vs` | Storage für `v` (nur bei `vt` = flow/global) |
| `v2` | Zweiter Wert (nur bei `btwn`, `idxbtwn`) |
| `v2t` | Typ für `v2` |
| `v2s` | Storage für `v2` |
| `case` | Bei `regex`/`cont`: Case-Sensitivity-Flag |

### Operator-Kürzel (`t`)

| Kürzel | Operator |
|---|---|
| `eq` / `neq` | `==` / `!=` |
| `lt` / `lte` / `gt` / `gte` | Vergleiche |
| `btwn` | `is between` |
| `cont` | `contains` |
| `regex` | `matches regex` |
| `true` / `false` | `is true` / `is false` |
| `null` / `nnull` | `is null` / `is not null` |
| `empty` / `nempty` | `is empty` / `is not empty` |
| `istype` | `is of type` (Typ in `v`) |
| `else` | `otherwise` (Catch-all) |

## Beispiele

### Routing nach Status-String
```
Property: msg.payload.status
  == "ok"      → Output 1
  == "warn"    → Output 2
  == "error"   → Output 3
  otherwise    → Output 4
```

### Schwellwert-Splitting
```
Property: msg.payload
  <  10        → Output 1 (low)
  is between 10..50  → Output 2 (mid)
  >  50        → Output 3 (high)
```

### Topic-Filter via Regex
```
Property: msg.topic
  matches regex "^sensor/temp/"  → Output 1
  matches regex "^sensor/hum/"   → Output 2
  otherwise                       → Output 3 (unbekannt)
```

### Existenz-Filter
```
Property: msg.payload.userId
  is not null  → Output 1 (verarbeiten)
  otherwise    → Output 2 (verwerfen / loggen)
```

## Implementierung

### Backend (`internal/nodes/switch.go`)

- **Inputs:** 1, **Outputs:** dynamisch = `len(rules)`
- Implementiert `flow.NodeInstance` und `flow.ContextProvider` (für flow/global Lookups)
- `Init()` parst `rules`, kompiliert ggf. Regex einmalig
- `OnMessage(msg)`:
  1. Property-Wert via Scope (msg/flow/global) holen
  2. Über Regeln iterieren — für jede zutreffende Regel `n.send(msg, outputIdx)`
  3. Im `stop after first match`-Modus nach erstem Treffer abbrechen
  4. `else` matched nur, wenn vorher nichts getroffen hat (auch im `checkall`-Modus)
- Defensive Kopie der Message **nur** wenn `checkall=true` und mehrere Outputs treffen — sonst Pointer-Pass

### Typkonvertierung

- Vergleiche nutzen die gleiche Konvertierungslogik wie Change Node (`valuetype.go`)
- `==`/`!=` mit loser Typkonvertierung (z.B. `"10" == 10` ist true)
- Strikte Vergleiche (`===`) bewusst weggelassen — kann später ergänzt werden falls gewünscht

### Frontend

#### `SwitchConfig.vue`
- Wiederverwendung der Bausteine aus `ChangeConfig.vue`:
  - `MsgFieldEditor` für Property-Wahl mit Scope+Storage
  - `ValueTypeInput` für die Vergleichswerte
  - `FormSelect` für Operator-Dropdown
- Sortierbare Regelliste (Drag & Drop) — Reihenfolge wird auf Port-Reihenfolge gemappt
- Modus-Toggle (Radio: stop after first / check all)

#### `SwitchNode.vue`
- BaseNode mit Category `function` (oder neue Kategorie `routing` falls farblich abgegrenzt werden soll)
- Body zeigt Property + Regelanzahl, z.B. `msg.payload · 4 rules`
- Dynamische Höhe je nach Output-Anzahl (siehe `NODE_AND_MULTIOUTPUT.md`)

### Node-Registrierung

```go
registry.Register("switch", nodes.NewSwitchNode, nodes.SwitchTypeInfo())
```

```go
func SwitchTypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:        "switch",
        Category:    "function",
        Label:       "Switch",
        Description: "Route messages based on property values",
        Icon:        "mdi-call-split",
        Defaults: map[string]any{
            "property":     "payload",
            "propertyType": "msg",
            "checkall":     false,
            "rules": []any{
                map[string]any{"id": "r1", "t": "eq", "v": "", "vt": "str"},
                map[string]any{"id": "r2", "t": "else"},
            },
        },
        Inputs:  1,
        Outputs: 2, // wird zur Laufzeit aus len(rules) abgeleitet
    }
}
```

> **Offen:** Wie wird ein Node-Type mit **dynamischer** Output-Anzahl registriert? Aktuell ist `Outputs` ein statisches Int. Das muss entweder über eine Funktion `OutputsFunc(props) int` oder durch Berechnung beim Editor-Save gelöst werden.

## Abhängigkeiten

- `flow.ContextStore` / `flow.ContextProvider` — vorhanden (vom Change Node verwendet)
- `msg.Get()` mit Dot-Path — vorhanden
- `valuetype.go` — Typkonvertierung wiederverwenden
- Multi-Output-Handling in der Engine — vorhanden (Function Node hat es), siehe `NODE_AND_MULTIOUTPUT.md` für offene UI-Probleme
- Frontend: `MsgFieldEditor`, `ValueTypeInput`, `FormSelect` aus `components/config/`

## Offene Fragen

1. **Dynamische Output-Anzahl** in `NodeTypeInfo` — Pattern abstimmen, ggf. eigenes Issue.
2. **`previous value`-Typ** — braucht Per-Node-State zwischen Messages. MVP oder weglassen?
3. **`is of type`** — Welche Typen werden tatsächlich unterstützt (Buffer? Date?).
4. **Strikte Vergleiche** (`===`/`!==`) — gewünscht oder bewusst weglassen?

> Sequenz-bezogene Operatoren (`head`/`tail`/`index between`) wurden bewusst weggelassen — siehe `SPLIT_JOIN_NODE.md`. Die setzen ein `msg.parts`-Konzept voraus, das Flint heute nicht hat.
