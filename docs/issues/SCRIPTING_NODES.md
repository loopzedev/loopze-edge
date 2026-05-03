# Issue: Scripting Nodes — JS / Go / Expr Function plus Expr in Change Node

## Status: Open

## Problembeschreibung

Der heutige Function-Node nutzt Goja (pure-Go JS-Interpreter, kein JIT). Für Logic-und-Glue ist das ideal — kompakt, sicher, single-binary. Für **datenbatch-orientierte Use-Cases** (10k+ Records aggregieren, binäres Parsen, Pipeline-Transforms) ist Goja aber spürbar langsam: gemessen ~100× langsamer als nativer Go-Code, mit ~11.000 Allokationen pro 1k-Record-Operation. Wenn jemand 10k Messages pro Sekunde durch komplexen Function-Code schiebt, wird der Function-Node zum Bottleneck.

Der Plan: **drei spezialisierte Function-Node-Typen** mit klar getrennten Zielgruppen, plus expr als inline-Tool im bestehenden Change-Node für einfache property-mutationen.

**Scope**: Architektur, neue Nodes und Change-Node-Erweiterung. v3-MQTT-Style-Themen wie Sandboxing-Hardening, Resource-Limits oder Hot-Reload sind ausdrücklich nicht hier.

## Übersicht

| Node | Type-ID | Engine | Use-Case | Performance ggü. nativem Go |
|---|---|---|---|---|
| **Function** (existing) | `function` | Goja (JS) | Logik, Glue, allgemeiner Code | ~100× langsamer |
| **Expr Function** (new) | `function-expr` | expr-lang/expr | Pipeline-Transforms, Aggregate, Filter | ~30-50× langsamer |
| **Go Function** (new) | `function-go` | traefik/yaegi | Algorithmen, Batch-Code, binäres Parsen | ~3-25× langsamer |
| **Change** (extended) | `change` | + expr-Resolver | Inline-Expressions in Property-Rules | n/a |

Die Performance-Zahlen stammen aus dem Benchmark in `benchmark/bench_test.go` (1k–100k Records, map- und typed-Shapes, AMD Ryzen 7 5800H).

## Anforderungen

### 1. Architektur — `internal/scripting` Package

Drei verschiedene Engines mit ähnlichem Compile-Then-Run-Lifecycle, aber unterschiedlichen Idiomen (JS-Function vs Pipeline-Expression vs Go-Function). Statt einer großen abstrakten Engine-Schnittstelle, die auf den kleinsten gemeinsamen Nenner reduziert, nehmen wir den **pragmatischen Mittelweg**:

- Geteilte **Utilities** in `internal/scripting`: Konvertierung von `flow.Message` ↔ Engine-Env, Buffer-`[]int` ↔ `[]byte` Bridges
- **Pro Engine** ein eigenes Sub-Package mit einer Engine-spezifischen Implementierung — nicht hinter einem gemeinsamen Interface zusammengeklemmt
- Jeder Function-Node hat seinen eigenen Code-Pfad und nutzt das Sub-Package seiner Engine

```
internal/scripting/
  ├─ scripting.go          # MessageEnv, MessageFromEnv, BufferToInts, IntsToBuffer
  ├─ goja/
  │  └─ engine.go          # Goja-Wrapper (extrahiert aus aktuellem function*.go)
  ├─ expr/
  │  └─ engine.go          # expr-Wrapper, Compile-At-Deploy
  └─ yaegi/
     └─ engine.go          # Yaegi-Wrapper, Funktion-Reflection
```

Begründung: die drei Engines unterscheiden sich semantisch zu sehr für ein gemeinsames Interface (Goja: Function mit Side-Effects via `node.send`; expr: Single-Expression mit Return-Value; Yaegi: Go-Function mit reflektierter Signatur). Eine gemeinsame Schnittstelle würde alle drei zwingen, sich auf den Goja-Kompromiss runterzuziehen.

### 2. Compile-At-Deploy für alle Engines

Heute compiled der Goja-Function-Node bereits in `Start()` (siehe `internal/nodes/function.go:108-124`). Das wird das Pattern für alle drei Engines:

- **Init/Start des Nodes** → Engine-spezifischer Compile (`goja.Compile`, `expr.Compile`, `yaegi.Eval`)
- **Compile-Errors werden Deploy-Errors** — der Node geht in Status "rot" mit der Fehlermeldung
- **Pro Message** nur noch Run/Call — keine Parsing-/Compile-Kosten

Wichtige UX-Konsequenz: Tippfehler im User-Code zeigen sich beim Deploy, nicht erst bei der ersten Message. Bei expr und Yaegi (statisch typisiert) gilt das auch für Type-Mismatches im deklarierten Env.

### 3. Expr Function Node (`function-expr`)

#### Zweck

Pipeline-Style-Transformationen — `map`/`filter`/`reduce`/`sum` über Listen, conditional Object-Construction, simple Aggregate. Idiomatisch eine einzige Expression, kein Control-Flow, keine Mutationen.

#### Konfiguration

- `expression` (string) — die expr-Expression. Operiert auf einem Env mit:
  - `payload` — der `msg.payload` der eingehenden Message
  - `topic` — der `msg.topic`
  - `msg` — die komplette Message als Map (Escape-Hatch für Felder außerhalb von payload/topic)
- `outputProperty` (string, default `"payload"`) — auf welches Message-Feld das Ergebnis geschrieben wird
- `passThrough` (boolean, default `false`) — wenn `true`, bleibt die ursprüngliche Message erhalten und nur `outputProperty` wird überschrieben; wenn `false`, wird eine neue Message mit nur dem Result + `topic` ausgegeben

#### Beispiele

```javascript
// Filter und Aggregate über Sensor-Readings
{
    avg: mean(map(payload, .temperature)),
    max: max(map(payload, .temperature)),
    above_threshold: count(filter(payload, .temperature > 25))
}
```

```javascript
// Konditionales Routing
filter(payload, .priority == "high")
```

```javascript
// Topic-basierte Logik mit Pipe-Syntax
payload | map({ time: .timestamp, value: .temperature * 1.8 + 32 })
```

#### Properties-Panel

```
┌──────────────────────────────────────────────┐
│  Expr Function                                │
├──────────────────────────────────────────────┤
│                                               │
│  Expression                                   │
│  ┌────────────────────────────────────────┐   │
│  │ sum(filter(payload, .temp > 20))       │   │
│  │                                        │   │
│  │ // weitere Zeilen erlaubt              │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Output Property                              │
│  ┌────────────────────────────────────────┐   │
│  │ payload                                │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  ☐ Pass through original message              │
│                                               │
└──────────────────────────────────────────────┘
```

Compile-Errors werden unter dem Editor inline angezeigt mit Zeilen-/Spalten-Marker.

### 4. Go Function Node (`function-go`)

#### Zweck

Algorithmen mit echtem Control-Flow (`for`, `if/else`, mutable State, mehrstufige Logik), die als einzelne expr-Expression nicht ausdrückbar sind. Insbesondere für **batch-numerische** Use-Cases, die in Goja zu langsam wären.

#### Konfiguration

- `code` (string) — Go-Code mit einer Funktion namens `handle`
- `outputs` (number, default `1`) — Anzahl Output-Ports (analog zum JS Function-Node)

#### Code-Konvention

Der User schreibt Go-Code mit einer Funktion `handle`, deren Signatur die Engine per Reflection liest:

```go
// Einfachste Form — payload als generischer Wert
func handle(payload any) any {
    // ...
    return payload
}
```

```go
// Mit typed Map-Access
func handle(payload []map[string]any) []map[string]any {
    out := []map[string]any{}
    for _, r := range payload {
        if t, ok := r["temperature"].(float64); ok && t > 25 {
            out = append(out, r)
        }
    }
    return out
}
```

```go
// Mit user-defined Struct (typed-Pfad, schnellster)
type Reading struct {
    Temperature float64 `json:"temperature"`
    Humidity    float64 `json:"humidity"`
}

func handle(payload []Reading) float64 {
    sum := 0.0
    for _, r := range payload {
        if r.Temperature > 20 {
            sum += r.Temperature*1.8 + 32
        }
    }
    return sum
}
```

Wenn der User typed Structs nimmt, **konvertiert die Engine an der Boundary** zwischen `flow.Message` und der Yaegi-Function (via JSON-Marshal/Unmarshal mit den `json:`-Tags). Das kostet Reflection — aber ist immer noch schneller als Goja-Map-Zugriff bei großen Batches, weil der Hot-Loop drinnen typed bleibt.

#### Buffer-Interop

Buffer-Payloads von mqtt-in kommen als `[]int`. Der Yaegi-Code arbeitet **direkt mit `[]byte`**:

```go
import "encoding/binary"

func handle(payload []byte) int64 {
    return int64(binary.BigEndian.Uint64(payload[:8]))
}
```

Die Engine konvertiert beim Eintritt `[]int` → `[]byte` und beim Austritt zurück. Über `internal/scripting.IntsToBuffer` und `BufferToInts`. Der User merkt davon nichts.

#### Multi-Output via `node.send`

Analog zur JS-Function bekommt der Yaegi-Code ein `node`-Objekt:

```go
func handle(payload any, node Node) {
    if /* condition */ {
        node.Send(0, payload)
    } else {
        node.Send(1, payload)
    }
}
```

`Node` wird als Interface ins Yaegi-Env exportiert, mit Methoden `Send(port int, msg any)`, `Log(args ...any)`, `Warn(args ...any)`, `Error(args ...any)`, `Status(fill, text string)`.

#### Properties-Panel

Identisch zur JS-Function: ein großer Code-Editor (mit Go-Syntax-Highlighting) plus ein `outputs`-Feld. Inline-Compile-Errors mit Zeilen-Marker.

### 5. Change Node — Expr als Value-Type

Der Change-Node hat heute Rules mit folgenden Value-Types: `msg`, `flow`, `global`, `str`, `num`, `bool`, `json`, `date`, `env`. Wir fügen einen weiteren hinzu: **`expr`**.

#### Verhalten

- Value-Type `expr` → der Value-Field-Inhalt ist eine expr-Expression
- Beim Deploy werden alle expr-Rules eines Change-Nodes vor-compiled
- Pro eingehender Message wird die Expression gegen ein Env mit `msg`/`flow`/`global` ausgewertet, das Ergebnis wird auf die Target-Property gesetzt

#### Beispiel

| Property | Value Type | Value |
|---|---|---|
| `payload` | `expr` | `payload * 1.8 + 32` |
| `topic` | `expr` | `topic + "/converted"` |
| `severity` | `expr` | `payload.value > 50 ? "high" : "low"` |

#### UX

Im Change-Node-Properties-Panel taucht `expr` neben den existierenden Value-Types im Dropdown auf. Bei Auswahl wird das Value-Field zu einem kleineren Code-Editor (Single-Line oder kleines Textarea) mit Syntax-Hint.

### 6. Naming und Palette

Alle drei Function-Nodes erscheinen in der Palette **getrennt**, mit klar unterscheidbaren Beschreibungen:

| Type-ID | Label (Palette) | Description |
|---|---|---|
| `function` | Function | JavaScript — logic and glue |
| `function-expr` | Expr Function | Single expression — fast pipelines, transforms |
| `function-go` | Go Function | Go code — fast batch processing |

**Bewusst kein "default"**: User wählt nach Use-Case, kein impliziter "fast" / "slow" Pfad.

## Datenstruktur

### workspace.json — Function-Expr Beispiel

```json
{
  "id": "node-expr-1",
  "type": "function-expr",
  "name": "Aggregate readings",
  "config": {
    "expression": "{ avg: mean(map(payload, .temperature)), max: max(map(payload, .temperature)) }",
    "outputProperty": "payload",
    "passThrough": false
  }
}
```

### workspace.json — Function-Go Beispiel

```json
{
  "id": "node-go-1",
  "type": "function-go",
  "name": "Filter sensors",
  "config": {
    "code": "type Reading struct {\n  Temperature float64 `json:\"temperature\"`\n}\n\nfunc handle(payload []Reading) []Reading {\n  out := []Reading{}\n  for _, r := range payload {\n    if r.Temperature > 25 { out = append(out, r) }\n  }\n  return out\n}",
    "outputs": 1
  }
}
```

### workspace.json — Change-Node mit expr-Rule

```json
{
  "id": "node-change-1",
  "type": "change",
  "config": {
    "rules": [
      {
        "action": "set",
        "property": "payload",
        "valueType": "expr",
        "value": "payload * 1.8 + 32"
      }
    ]
  }
}
```

## Betroffene Dateien

### Backend — Neu

- `internal/scripting/scripting.go` — gemeinsame Utilities (`MessageEnv`, `MessageFromEnv`, `BufferToInts`, `IntsToBuffer`)
- `internal/scripting/goja/engine.go` — extrahierter Goja-Code (Refactor von `internal/nodes/function*.go`)
- `internal/scripting/expr/engine.go` — expr-Wrapper mit `Compile`/`Run`-Methoden
- `internal/scripting/yaegi/engine.go` — Yaegi-Wrapper mit Reflection-basierter Signatur-Erkennung und Boundary-Konvertierung
- `internal/nodes/function_expr.go` — Expr Function Node
- `internal/nodes/function_go.go` — Go Function Node
- `internal/nodes/function_expr_test.go` + `function_go_test.go` — Tests pro Node
- `frontend/src/components/config/ExprFunctionConfig.vue`
- `frontend/src/components/config/GoFunctionConfig.vue`

### Backend — Anpassungen

- `internal/nodes/function.go` und `function_buffer.go` — refactored um `internal/scripting/goja` zu nutzen, kein UX-Change
- `internal/nodes/change.go` — neuer Value-Type `expr`, Compile-Loop in `Init()` für alle expr-Rules
- `internal/server/server.go` — Registrierung von `function-expr` und `function-go`

### Frontend — Anpassungen

- `frontend/src/components/config/ChangeNodeConfig.vue` (oder wie sie heißt) — `expr` als Value-Type-Option im Dropdown, Code-Editor-Variante für das Value-Field bei `expr`
- `frontend/src/components/PropertyPanel.vue` — Dispatch für die zwei neuen Node-Types
- `frontend/src/components/nodes/tokens.ts` — Palette-Tokens für `function-expr` (z.B. blaue Pipeline-Ikone) und `function-go` (Go-Gopher oder fast-forward)

### Go Dependencies

- `github.com/expr-lang/expr` — bereits validiert im Benchmark
- `github.com/traefik/yaegi` — bereits validiert im Benchmark

## Technische Hinweise

### Engine-Lifecycle pro Node

Beide neuen Engines folgen dem gleichen Pattern wie der existierende Goja-Function-Node:

```go
// In Init(): nur Properties parsen
func (n *ExprFunctionNode) Init() error {
    n.expression, _ = n.config.Properties["expression"].(string)
    // ... weitere Felder
    return nil
}

// In Start(): compile, error → red status
func (n *ExprFunctionNode) Start() error {
    program, err := n.engine.Compile(n.expression, envShape)
    if err != nil {
        n.status("red", "compile: " + err.Error())
        return err
    }
    n.program = program
    return nil
}

// Pro Message: nur run
func (n *ExprFunctionNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
    env := scripting.MessageEnv(msg)
    result, err := n.program.Run(env)
    if err != nil {
        return nil, err
    }
    out := scripting.MessageFromEnv(env, result, n.outputProperty, n.passThrough)
    return [][]*flow.Message{{out}}, nil
}
```

### Yaegi — Boundary-Conversion bei typed Structs

Wenn die User-Function `[]Reading` als Input erwartet aber `flow.Message.Payload()` ein `[]map[string]any` liefert, brauchen wir eine Conversion. Drei Optionen:

1. **JSON-Marshal/Unmarshal** — pragmatisch, ~100% Reflection-Cost, aber idiomatisch via `json:`-Tags
2. **Reflection direkt** — würden uns vor jedes Field-Mapping setzen, schneller aber komplex
3. **Codegen** — Buildtime-Generation eines Convertierers, schnellst, aber riesiger UX-Aufwand

Wir nehmen (1) — JSON-Marshal/Unmarshal. Über `json:`-Tags hat der User volle Kontrolle, der Code ist klein. Performance: für 10k Records mit 3 Feldern liegt die Conversion bei ~1-2ms. Das ist relevant aber nicht prohibitiv — der Yaegi-Hot-Loop im typed-Pfad ist immer noch schneller als Goja im map-Pfad.

```go
// Engine-seitig:
payload := msg.Payload()
inputType := reflect.TypeOf(handle).In(0)        // z.B. []Reading
typed := reflect.New(inputType).Interface()
data, _ := json.Marshal(payload)                 // []byte
json.Unmarshal(data, typed)                      // populated []Reading
result := callHandle(typed)                      // schnell, typed
```

### Yaegi — Code-Sandbox

Yaegi exposed by default die Go-Standardlibrary via `i.Use(stdlib.Symbols)`. Das gibt User-Code Zugriff auf `os.Open`, `net.Dial` etc. — was wir **nicht wollen**.

Wir registrieren explizit nur ein **gefiltertes Subset**:
- `encoding/binary` — binäres Parsen
- `encoding/json` — JSON-Parsen
- `fmt` (nur `Sprintf`, `Sprint`, kein `Println`)
- `math`, `math/big`
- `strings`, `strconv`, `bytes`
- `sort`
- `time` (nur Parsing/Formatting, kein `Sleep` etc.)

Verboten: `os`, `io/ioutil`, `net`, `net/http`, `runtime`, `unsafe`, `syscall`. Wenn der User-Code das importiert, schlägt der Compile fehl.

Liste wird in `internal/scripting/yaegi/symbols.go` als kuratiertes Map gepflegt.

### Expr — Env-Shape und Type-Validation

`expr.Compile` akzeptiert eine Env-Shape:

```go
env := map[string]any{
    "payload": []any{},          // generisch — "payload ist eine Liste"
    "topic":   "",
    "msg":     map[string]any{},
}
program, err := expr.Compile(userExpression, expr.Env(env))
```

Das gibt dem Compiler genug Info um Field-Access (`payload[0].x`) zu validieren, aber lässt den User flexibel. Wenn der User typed-Strenge will, kann er per Doc-Comment oben in der Expression eine Hint geben — aktuelle Version: einfache `any`-basierte Env, später erweiterbar mit explicit-typed-shape-Property im Node.

### Change-Node — expr-Compile-Lifecycle

Alle Rules mit `valueType: "expr"` werden in `Init()` des Change-Nodes vor-compiled. Compile-Fehler einer einzelnen Rule machen den ganzen Node zu einem Deploy-Error (mit klarer Lokalisierung welche Rule).

```go
func (n *ChangeNode) Init() error {
    for i, rule := range n.rules {
        if rule.ValueType == "expr" {
            prog, err := expr.Compile(rule.Value, expr.Env(...))
            if err != nil {
                return fmt.Errorf("change rule %d: %w", i, err)
            }
            n.compiledRules[i] = prog
        }
    }
    return nil
}
```

### Frontend — geteilter Code-Editor

Alle drei Function-Nodes (JS, expr, Go) plus die expr-Rules im Change-Node nutzen **denselben Code-Editor-Wrapper** (`SimpleEditor.vue` aktuell, auf Monaco-Basis). Unterschied: die Sprachen-Konfiguration:
- JS Function → `language: 'javascript'`
- Go Function → `language: 'go'`
- Expr Function → `language: 'expr'` (custom Monaco-Tokenizer, simpel — Keywords + Operatoren, kein Linter)

Compile-Errors werden als Inline-Decorations am betroffenen Zeilen-Marker angezeigt. Die Engine liefert beim Compile-Fehler Position-Infos, die Vue-Komponente reicht das an Monaco weiter.

## Performance — was wir versprechen

Aus dem Benchmark (`benchmark/bench_test.go`, AMD Ryzen 7 5800H, 1k bis 100k Records):

| Operation | Native Go | Goja (JS) | Expr | Yaegi (map) | Yaegi (typed) |
|---|---:|---:|---:|---:|---:|
| 1k Records | 7.7 µs | 752 µs | 321 µs | 192 µs | 77 µs |
| 10k Records | 80 µs | 6.87 ms | 3.16 ms | 1.77 ms | 773 µs |
| 100k Records | 1.5 ms | 64.7 ms | 29.5 ms | 20.5 ms | 7.83 ms |
| **Speedup ggü Goja @10k** | 86× | 1× | **2.2×** | **3.9×** | **8.9×** |

**Realismus**: das sind Pure-Computation-Zahlen. In der Praxis kommen IO-Latenzen, MQTT-Roundtrips, Workspace-Sync etc. dazu. Die Engine-Wahl macht im IO-bound Hot-Path 0% Unterschied, im CPU-bound Hot-Path ist der Faktor wie oben.

## Abhängigkeiten

- Keine harte Abhängigkeit zu anderen Issues
- Profitiert vom v5-MQTT-Setup (`buffer`-Output-Mode liefert `[]int`, das Yaegi und Goja als `[]byte` interpretieren können — siehe Buffer-Interop)
- Setzt das Config-Node-Konzept aus dem MQTT-Issue voraus (für eventuelle künftige Engines die selbst Config-Nodes brauchen — nicht hier, aber gut zu wissen)

## Abgrenzung / Nicht im Scope

- **WASM-Function-Node** — eine vierte Engine via wazero für maximale Performance bei Compute-Heavy-Workloads. Sinnvoll, aber separater Issue mit eigener UX (Upload `.wasm`, kein Editor)
- **V8-Bindings (cgo)** — widerspricht dem Single-Binary-Charakter von LOOPZE
- **Node.js Subprocess** — gibt Self-Contained-Deployment auf
- **Reactive Expressions** — expr-Programs, die bei Änderung einer Source-Variable neu evaluiert werden ohne Trigger-Message. Interessant für Live-UI-Bindungen, aber nicht das Function-Node-Modell
- **Hot-Reload von User-Code ohne Re-Deploy** — wäre ein editor-experience-Boost, ist aber technisch komplex (engine-Lifecycle, Subscription-Re-Wiring, State-Migration)
- **Multi-Tenancy / strenges Sandboxing** — Resource-Limits (CPU-Time, Memory), Secrets-Hiding etc. werden im Credential-System adressiert, nicht hier
- **Code-Sharing zwischen Function-Nodes** — keine `import` von einem Function-Node in einen anderen. Wenn nötig: zukünftiges "Library-Node"-Konzept
- **Auto-Engine-Wahl** — kein "Magic Function-Node, der je nach Input Goja/expr/Yaegi auswählt". User wählt explizit
- **Type-Inference für Yaegi-Schema aus Sample-Messages** — wäre nett aber separater UX-Wurf
