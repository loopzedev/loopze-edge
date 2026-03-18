# Change Node — Implementierungsplan

## Context

Der Change Node ist der zweite Core-Processing-Node nach dem Function Node. Er ermöglicht Datenmanipulation ohne Code — Properties setzen, ändern, löschen, verschieben. Spezifikation: `issues/CHANGENODE.md`.

Viel Infrastruktur existiert bereits: Icon, Token-Kategorie, FlowEditor-Slot, ContextProvider-Interface, Message dot-path API.

---

## Neue Dateien

### 1. `internal/nodes/change.go` (~400 Zeilen)

**Struct:**
```go
type Rule struct {
    Type     string // "set", "change", "delete", "move"
    Property string // Ziel-Property (ohne Scope)
    PropType string // "msg", "flow", "global"
    To       string // Wert oder Ziel-Property
    ToType   string // "msg", "flow", "global", "str", "num", "bool", "json", "date", "env"
    From     string // Suchstring (nur bei "change")
    FromType string // "str", "re", "num", "bool", "env"
    FromRE   bool   // Regex-Flag
}

type ChangeNode struct {
    config   flow.NodeConfig
    send     flow.SendFunc
    status   flow.StatusFunc
    debug    flow.DebugFunc
    ctxMem, ctxPers     flow.ContextStore  // global
    flowMem, flowPers   flow.ContextStore  // flow-scoped
    rules    []Rule
}
```

**Init():** Parst `rules` Array aus `config.Properties`. Jede Rule wird aus `map[string]any` gemappt. Bei `fromRE: true` wird der Regex vorkompiliert (`regexp.Compile`).

**HandleMessage():**
```
for each rule:
  1. Resolve target value (resolveValue):
     - "str" → use literal
     - "num" → strconv.ParseFloat
     - "bool" → literal true/false
     - "json" → json.Unmarshal
     - "date" → time.Now().UnixMilli()
     - "env" → os.Getenv()
     - "msg" → msg.Get(path)
     - "flow" → flowMem.Get(key)
     - "global" → ctxMem.Get(key)

  2. Apply operation:
     - "set":
       - pt=msg → msg.Set(property, value)
       - pt=flow → flowMem.Set(property, value)
       - pt=global → ctxMem.Set(property, value)
     - "delete":
       - pt=msg → msg.Delete(property)
       - pt=flow → flowMem.Delete(property)
       - pt=global → ctxMem.Delete(property)
     - "move":
       - Get from source → Set at target → Delete source
     - "change":
       - Get current string value
       - fromRE=true → regexp.ReplaceAllString
       - fromRE=false → strings.ReplaceAll
       - Set modified value back

Return message on port 0.
```

**SetContext():** Implementiert `flow.ContextProvider` (identisch zu FunctionNode).

**ChangeTypeInfo():**
```go
Type: "change", Category: "function", Label: "Change"
Inputs: 1, Outputs: 1
Defaults: { rules: [{ t:"set", p:"payload", pt:"msg", to:"", tot:"str" }] }
```

Existierende APIs die wiederverwendet werden:
- `msg.Get/Set/Delete` (`internal/flow/types.go:145-218`)
- `ContextStore.Get/Set/Delete` (`internal/flow/context.go:8-20`)
- `ContextProvider` Interface (`internal/flow/context.go:31-33`)

### 2. `internal/nodes/change_test.go` (~300 Zeilen)

Tests für jede Operation:
- `TestChangeNode_SetMsgProperty` — Setze msg.payload auf String
- `TestChangeNode_SetNested` — Setze msg.data.nested.field
- `TestChangeNode_SetFromMsg` — Kopiere msg.topic → msg.payload
- `TestChangeNode_SetTimestamp` — Setze auf aktuellen Timestamp
- `TestChangeNode_SetJSON` — Setze auf JSON-Objekt
- `TestChangeNode_SetNumber` — Setze auf Zahl
- `TestChangeNode_SetBoolean` — Setze auf Boolean
- `TestChangeNode_Delete` — Lösche Property
- `TestChangeNode_Move` — Verschiebe Property
- `TestChangeNode_ChangeString` — Suche/Ersetze String
- `TestChangeNode_ChangeRegex` — Suche/Ersetze Regex
- `TestChangeNode_MultipleRules` — Mehrere Regeln sequenziell
- `TestChangeNode_FlowContext` — Setze/Lese flow.* Context
- `TestChangeNode_GlobalContext` — Setze/Lese global.* Context

### 3. `frontend/src/components/config/ChangeConfig.vue` (~400 Zeilen)

Komplexeste Config-Komponente — Regelliste mit dynamischem Layout je nach Operation.

**Struktur pro Regel:**
```
┌─────────────────────────────────────────────────┐
│ ≡  [Operation ▼]  [▼ scope] [property    ]  ✕  │
│                                                  │
│    (bei "set":)                                  │
│    to the value   [▼ typ] [value         ]      │
│                                                  │
│    (bei "change":)                               │
│    Suche nach     [▼ typ] [search        ]      │
│    Ersetze durch  [▼ typ] [replace       ]      │
│                                                  │
│    (bei "move":)                                 │
│    to              [▼ scope] [property   ]      │
│                                                  │
│    (bei "delete": keine Zusatzfelder)            │
└─────────────────────────────────────────────────┘
```

**Komponenten-Design:**
- `rules` als computed Array aus `config.rules`
- `updateRule(index, field, value)` — aktualisiert einzelnes Feld
- `addRule()` — fügt Default-Regel hinzu
- `removeRule(index)` — entfernt Regel
- `moveRule(from, to)` — verschiebt Regel (Drag & Drop oder Up/Down Buttons)

**Dropdowns:**
- Operation: `set | change | delete | move`
- Scope: `msg. | flow. | global.`
- Value-Typ (set): `msg. | flow. | global. | string | number | boolean | JSON | buffer | timestamp | Umgebungsvariable`
- Search-Typ (change): `msg. | flow. | global. | string | Regulärer Ausdruck | number | boolean | Umgebungsvariable`

**Styling:** Konsistent mit InjectConfig.vue — `terminal-input`, `terminal-border`, `text-terminal-text-dim`, Button-Toggles wie in ContextWatchConfig.

### 4. `frontend/src/components/nodes/ChangeNode.vue` (~30 Zeilen)

Einfacher Wrapper um BaseNode:
```vue
<BaseNode node-type="change" :inputs="1" :outputs="1">
  <template #body>
    <span>{{ ruleCount }} rule{{ ruleCount !== 1 ? 's' : '' }}</span>
  </template>
</BaseNode>
```

---

## Bestehende Dateien — Änderungen

### 5. `internal/server/server.go` (1 Zeile)

```go
// In registerNodes(), nach context-watch:
registry.Register("change", nodes.NewChangeNode, nodes.ChangeTypeInfo())
```

### 6. `frontend/src/components/PropertyPanel.vue` (2 Zeilen)

```vue
// Import hinzufügen:
import ChangeConfig from '@/components/config/ChangeConfig.vue'

// Condition hinzufügen nach ContextWatchConfig:
<ChangeConfig v-else-if="selectedNode?.type === 'change'" />
```

### 7. `frontend/src/views/FlowEditor.vue` (3 Zeilen)

Existierender Slot (`<BaseNode v-bind="nodeProps as any" />`) ersetzen durch:
```vue
import ChangeNode from "@/components/nodes/ChangeNode.vue";

<template #node-change="nodeProps">
    <ChangeNode v-bind="nodeProps as any" />
</template>
```

### 8. `frontend/src/components/nodes/BaseNode.vue` (1 Zeile)

TypeLabel-Map erweitern (falls nicht schon vorhanden — prüfen):
```typescript
'change': 'Change',
```

---

## Bereits vorhanden (kein Handlungsbedarf)

- `tokens.ts`: `change: 'process'` → blaue Farben ✅
- `NodeIcon.vue`: `change` Icon (bidirektionale Pfeile) ✅
- `FlowEditor.vue`: `#node-change` Template-Slot ✅
- `ContextProvider` Interface ✅
- `msg.Get/Set/Delete` mit dot-path Navigation ✅

---

## Implementierungsreihenfolge

1. **Backend: `change.go`** — Rule-Struct, Init, HandleMessage mit allen 4 Operationen
2. **Backend: `change_test.go`** — Tests für alle Operationen
3. **Backend: `server.go`** — Registrierung
4. **`go build && go test`** — Backend verifizieren
5. **Frontend: `ChangeNode.vue`** — Node-Komponente
6. **Frontend: `ChangeConfig.vue`** — Config-Panel mit Regelliste
7. **Frontend: `PropertyPanel.vue` + `FlowEditor.vue`** — Verdrahtung
8. **`npm run type-check`** — Frontend verifizieren

---

## Verifikation

### Backend
```bash
go build ./...
go test -run TestChangeNode -v ./internal/nodes/
```

### Frontend
```bash
cd frontend && npm run type-check
```

### End-to-End
1. Flint starten (`make build-all && ./bin/flint`)
2. Change Node in Flow ziehen
3. Properties öffnen → Regeln konfigurieren:
   - Setze msg.payload auf "Hello"
   - Lösche msg.topic
4. Inject → Change → Debug verbinden
5. Deploy + Inject triggern
6. Debug-Panel: Message hat payload="Hello", kein topic
