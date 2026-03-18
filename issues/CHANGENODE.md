# Change Node

## Beschreibung

Der Change Node manipuliert Message-Properties, Flow-Context und Global-Context ohne Code schreiben zu müssen. Er ist das Hauptwerkzeug für einfache Datenmanipulationen und ersetzt in vielen Fällen einen Function Node.

## Operationen

Jede Regel besteht aus einer **Operation**, einem **Ziel** und (je nach Operation) einem **Wert**.

### Operationen (Dropdown links)

| Operation | Beschreibung |
|---|---|
| **Setze** | Setzt ein Property auf einen Wert. Erstellt es wenn es nicht existiert. |
| **Ändere** | Sucht und ersetzt Text innerhalb eines String-Properties (Regex oder String-Match). |
| **Lösche** | Entfernt ein Property komplett aus dem Objekt. |
| **Verschiebe** | Verschiebt ein Property an eine andere Stelle (Quelle wird gelöscht). |

### Ziel-Scope (Dropdown im Property-Feld)

| Scope | Beschreibung |
|---|---|
| **msg.** | Message-Property (z.B. `msg.payload`, `msg.topic`, `msg.myField`) |
| **flow.** | Flow-Context (Key-Value Store, geteilt innerhalb des Flows) |
| **global.** | Global-Context (Key-Value Store, geteilt über alle Flows) |

### Wert-Typen (Dropdown im Value-Feld)

Beim **Setze**-Operator kann der Wert aus verschiedenen Quellen kommen:

| Typ | Icon | Beschreibung |
|---|---|---|
| **msg.** | — | Wert aus einem anderen Message-Property lesen |
| **flow.** | — | Wert aus dem Flow-Context lesen |
| **global.** | — | Wert aus dem Global-Context lesen |
| **string** | `a/z` | Statischer String-Wert |
| **number** | `0/9` | Statischer Zahlenwert |
| **boolean** | `◉` | `true` oder `false` |
| **JSON** | `{}` | JSON-Objekt oder Array (wird geparst) |
| **buffer** | `01/10` | Buffer/Byte-Array |
| **timestamp** | `⏱` | Aktueller Unix-Timestamp in Millisekunden |
| **Umgebungsvariable** | `$` | Wert aus einer Umgebungsvariable lesen |

## Regeln-UI

- Regeln werden als **sortierbare Liste** dargestellt (Drag-Handle `≡` links)
- Jede Regel hat einen **Löschen-Button** (`✕`) rechts
- Unten ein **"+ hinzufügen"** Button für neue Regeln
- Regeln werden **sequenziell** von oben nach unten ausgeführt
- Änderungen einer Regel sind für nachfolgende Regeln sichtbar

## Beispiele

### Setze msg.payload auf einen String
```
Setze | msg.payload | to the value | string: "Hello World"
```

### Kopiere msg.topic nach msg.payload
```
Setze | msg.payload | to the value | msg.topic
```

### Lösche ein Property
```
Lösche | msg.temp
```

### Verschiebe Property
```
Verschiebe | msg.payload | to | msg.data.original
```

### Suchen & Ersetzen (Ändere)

Die Ändere-Operation hat ein **eigenes Layout** mit zwei Wert-Feldern:

```
Ändere | ▼ msg. [property]
          Suche nach:    | ▼ [typ] [wert]
          Ersetze durch: | ▼ [typ] [wert]
```

**"Suche nach"-Typen** (eingeschränkt):

| Typ | Beschreibung |
|---|---|
| **msg.** | Suchstring aus Message-Property |
| **flow.** | Suchstring aus Flow-Context |
| **global.** | Suchstring aus Global-Context |
| **string** | Statischer Suchstring |
| **Regulärer Ausdruck** | Regex-Pattern (z.B. `foo\d+`) |
| **number** | Zahlenwert |
| **boolean** | true/false |
| **Umgebungsvariable** | Suchstring aus ENV |

**"Ersetze durch"-Typen**: identisch, aber ohne "Regulärer Ausdruck".

Beispiel:
```
Ändere | msg.payload | Suche nach: string "foo" | Ersetze durch: string "bar"
```

### Timestamp setzen
```
Setze | msg.timestamp | to the value | timestamp
```

### Flow-Context schreiben
```
Setze | flow.lastValue | to the value | msg.payload
```

## Konfiguration (Backend)

```json
{
  "rules": [
    {
      "t": "set",
      "p": "payload",
      "pt": "msg",
      "to": "Hello World",
      "tot": "str"
    },
    {
      "t": "change",
      "p": "payload",
      "pt": "msg",
      "from": "foo",
      "fromt": "str",
      "to": "bar",
      "tot": "str",
      "fromRE": false
    },
    {
      "t": "delete",
      "p": "temp",
      "pt": "msg"
    },
    {
      "t": "move",
      "p": "payload",
      "pt": "msg",
      "to": "data.original",
      "tot": "msg"
    }
  ]
}
```

### Regel-Felder

| Feld | Beschreibung |
|---|---|
| `t` | Operation: `set`, `change`, `delete`, `move` |
| `p` | Property-Name (ohne Scope-Prefix) |
| `pt` | Property-Scope: `msg`, `flow`, `global` |
| `to` | Zielwert oder Ziel-Property |
| `tot` | Wert-Typ: `msg`, `flow`, `global`, `str`, `num`, `bool`, `json`, `buf`, `date`, `env` |
| `from` | Suchstring (nur bei `change`) |
| `fromt` | Such-Typ: `str`, `re` (Regex) |
| `fromRE` | Regex-Flag (nur bei `change`) |

## Implementierung

### Backend (`internal/nodes/change.go`)

- **Inputs:** 1, **Outputs:** 1
- Parst `rules` Array aus `config.Properties`
- Führt Regeln sequenziell auf der Message aus
- Für `msg.*`: Nutzt `msg.Get()` / `msg.Set()` / `msg.Delete()`
- Für `flow.*` / `global.*`: Nutzt `ContextStore.Get()` / `ContextStore.Set()`
- Implementiert `ContextProvider` (wie FunctionNode) für Flow/Global Context Zugriff
- Bei `change` (Suche/Ersetze): `strings.Replace()` oder `regexp.ReplaceAllString()`
- Bei `move`: Get → Set am Ziel → Delete an der Quelle
- Bei `date` (timestamp): `time.Now().UnixMilli()`
- Bei `env`: `os.Getenv()`

### Frontend

#### `ChangeConfig.vue`
- Sortierbare Regelliste (Drag & Drop Reihenfolge)
- Pro Regel: Operation-Dropdown, Scope-Dropdown, Property-Input, Value-Type-Dropdown, Value-Input
- "+" Button unten zum Hinzufügen neuer Regeln
- "✕" Button rechts zum Löschen einer Regel

#### `ChangeNode.vue`
- Nutzt BaseNode mit Category `process` (blaue Farben)
- Body zeigt kompakte Zusammenfassung der Regeln (z.B. "3 rules")

### Node-Registrierung

```go
registry.Register("change", nodes.NewChangeNode, nodes.ChangeTypeInfo())
```

```go
func ChangeTypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:        "change",
        Category:    "function",
        Label:       "Change",
        Description: "Set, change, delete or move message properties",
        Icon:        "mdi-pencil",
        Defaults: map[string]any{
            "rules": []any{
                map[string]any{
                    "t": "set", "p": "payload", "pt": "msg",
                    "to": "", "tot": "str",
                },
            },
        },
        Inputs:  1,
        Outputs: 1,
    }
}
```

## Abhängigkeiten

- `flow.ContextStore` Interface (existiert bereits)
- `flow.ContextProvider` Interface (existiert bereits)
- `msg.Get()` / `msg.Set()` / `msg.Delete()` (existiert bereits)
- Dot-Path Navigation in `msg.Get/Set` (existiert bereits)
