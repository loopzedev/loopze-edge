# Issue: Inject Node — Rules-System wie Change Node

## Status: Open

## Problembeschreibung

Der Inject Node hat aktuell hardcoded `payload` + `topic` Felder. Der User kann nur eine Payload und ein Topic konfigurieren. Node-RED's Inject Node erlaubt dagegen eine **dynamische Liste von Properties**, die auf der Message gesetzt werden — genau wie die "Set"-Regeln des Change Nodes.

**Ziel:** Der Inject Node bekommt ein Rules-System analog zum Change Node. Jede Rule definiert ein Property das auf der ausgehenden Message gesetzt wird. Rules können per Drag & Drop umsortiert, hinzugefügt und entfernt werden.

## Aktuell (wird ersetzt)

```json
{
  "once": false,
  "interval": 0,
  "payloadType": "date",
  "payload": "rfc3339",
  "topic": "my/topic",
  "topicType": "str"
}
```

Hardcoded: genau 1 Payload + 1 Topic, nicht erweiterbar.

## Neu: Rules-basierte Property-Liste

### Konfiguration

```json
{
  "once": false,
  "interval": 0,
  "props": [
    { "p": "payload", "vt": "date", "v": "rfc3339" },
    { "p": "topic",   "vt": "str",  "v": "sensors/temperature" },
    { "p": "qos",     "vt": "num",  "v": "1" }
  ]
}
```

Jede Rule in `props` definiert:

| Feld | Beschreibung |
|---|---|
| `p` | Property-Name auf `msg` (z.B. `payload`, `topic`, `qos`, `retain`, beliebig) |
| `vt` | Value-Type: `str`, `num`, `bool`, `json`, `date`, `env`, `flow`, `global` |
| `v` | Wert (abhängig vom Type) |
| `vs` | Storage: `memory` oder `persistent` (nur bei `flow`/`global`) |

**Kein `msg`-Type** — der Inject Node hat keine eingehende Message.

### Default-Rules

Neuer Inject Node startet mit zwei Default-Rules:

```json
[
  { "p": "payload", "vt": "date", "v": "rfc3339" },
  { "p": "topic",   "vt": "str",  "v": "" }
]
```

### Unterstützte Value-Typen

| Typ | `vt` | `v` Inhalt | Beschreibung |
|---|---|---|---|
| **String** | `str` | `"Hallo Welt"` | Statischer String |
| **Number** | `num` | `"42.5"` | Numerischer Wert (float64) |
| **Boolean** | `bool` | `"true"` | true/false |
| **JSON** | `json` | `'{"key": "val"}'` | JSON-Objekt oder Array |
| **Timestamp** | `date` | `"rfc3339"` oder `"epoch"` | Aktueller Zeitstempel |
| **Env-Variable** | `env` | `"MY_VAR"` | Umgebungsvariable auslesen |
| **Flow Context** | `flow` | `"myKey"` | Wert aus Flow-Context lesen |
| **Global Context** | `global` | `"myKey"` | Wert aus Global-Context lesen |

## Umsetzung

### Backend (`inject.go`)

Die `emit()` Methode iteriert über alle Rules und setzt die Properties auf der Message:

```go
func (n *InjectNode) emit() {
    msg := flow.NewMessage()
    for _, rule := range n.props {
        val, err := ResolveValue(rule.vt, rule.v, rule.vs, nil, n.valueContext())
        if err != nil {
            slog.Warn("inject: resolve error", "prop", rule.p, "error", err)
            continue
        }
        if val != nil {
            msg.Set(rule.p, val)
        }
    }
    n.send(0, msg)
}
```

**Config-Parsing:** `Init()` liest `props` als `[]map[string]any` (analog zu `rules` im Change Node).

**ContextProvider:** Bereits implementiert — InjectNode implementiert `SetContext()`.

### Shared Value-Type Komponente (Frontend)

**Bereits vorhanden:** `ValueTypeInput.vue` — wird wiederverwendet.

### Frontend (`InjectConfig.vue`)

Das Property-Panel bekommt die gleiche Rule-Liste wie der Change Node:

```
┌─────────────────────────────────────────────────┐
│ ☐ Inject once at startup                        │
│                                                  │
│ Repeat Interval                                  │
│ [None] [100ms] [500ms] [1s] [5s] [10s] ...     │
│ [________] ms                                    │
│                                                  │
│ Properties                                       │
│ ┌───────────────────────────────────────────┐   │
│ │ ≡  msg.[payload ] [timestamp▾] [rfc3339▾] │   │
│ │ ≡  msg.[topic   ] [string   ▾] [________] │   │
│ └───────────────────────────────────────────┘   │
│ [+ add property]                                 │
└─────────────────────────────────────────────────┘
```

Jede Zeile enthält:
- **Drag Handle** (`≡`) für Reihenfolge per Drag & Drop
- **Property Name** (FormInput, z.B. `payload`, `topic`, `qos`)
- **ValueTypeInput** (Type-Dropdown + Value-Input + optional Storage)
- **Delete Button** (`✕`)

Die Drag & Drop Logik wird aus `ChangeConfig.vue` wiederverwendet (gleiche `onDragStart`/`onDragOver`/`onDrop` Pattern).

### Shared Value-Resolution (Backend)

**Bereits vorhanden:** `valuetype.go` mit `ResolveValue()` — wird wiederverwendet.

### Bestehende Funktionalität (bleibt unverändert)

- Trigger-Modi: Manual, Once, Interval, Once+Interval
- TRIG-Button im Canvas
- API-Endpoint `POST /api/v1/inject/{id}`
- Node-Darstellung im Canvas (InjectNode.vue)
- Intervall-Presets im Property-Panel

## Betroffene Dateien

| Datei | Änderung |
|---|---|
| `internal/nodes/inject.go` | Rules-Parsing statt hardcoded payload/topic, `emit()` iteriert Rules |
| `internal/nodes/inject_test.go` | Tests für Multi-Rule, Reihenfolge, verschiedene Value-Typen |
| `frontend/src/components/config/InjectConfig.vue` | Rule-Liste mit Drag & Drop, nutzt `ValueTypeInput` |

## Tests

| Test | Prüft |
|---|---|
| `TestInjectSingleRule` | Eine Rule: payload=str |
| `TestInjectMultipleRules` | Mehrere Rules: payload + topic + custom property |
| `TestInjectRuleOrder` | Reihenfolge der Rules wird eingehalten |
| `TestInjectDefaultRules` | Ohne Props-Config → Default payload=timestamp |
| `TestInjectPayloadDateEpoch` | date-Type mit epoch |
| `TestInjectPayloadJSON` | json-Type |
| `TestInjectPayloadEnv` | env-Type |
| `TestInjectEmptyRules` | Leere props-Liste → Message ohne Properties |
