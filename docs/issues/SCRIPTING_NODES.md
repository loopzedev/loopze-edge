# Issue: Scripting Nodes — JS / Go / Expr Function plus Expr in Change Node

## Status: Open

## Problem description

Today's Function node uses Goja (pure-Go JS interpreter, no JIT). For logic-and-glue this is ideal — compact, safe, single-binary. For **data-batch-oriented use cases** (aggregating 10k+ records, binary parsing, pipeline transforms), Goja is noticeably slow: measured at ~100x slower than native Go code, with ~11,000 allocations per 1k-record operation. If someone pushes 10k messages per second through complex Function code, the Function node becomes the bottleneck.

The plan: **three specialized Function node types** with clearly separated target users, plus expr as an inline tool in the existing Change node for simple property mutations.

**Scope**: architecture, new nodes, and the Change node extension. v3-MQTT-style topics like sandboxing hardening, resource limits, or hot reload are explicitly not in scope here.

## Overview

| Node | Type ID | Engine | Use case | Performance vs. native Go |
|---|---|---|---|---|
| **Function** (existing) | `function` | Goja (JS) | Logic, glue, general code | ~100x slower |
| **Expr Function** (new) | `function-expr` | expr-lang/expr | Pipeline transforms, aggregates, filters | ~30-50x slower |
| **Go Function** (new) | `function-go` | traefik/yaegi | Algorithms, batch code, binary parsing | ~3-25x slower |
| **Change** (extended) | `change` | + expr resolver | Inline expressions in property rules | n/a |

The performance numbers come from the benchmark in `benchmark/bench_test.go` (1k–100k records, map and typed shapes, AMD Ryzen 7 5800H).

## Requirements

### 1. Architecture — `internal/scripting` package

Three different engines with a similar compile-then-run lifecycle, but different idioms (JS function vs pipeline expression vs Go function). Instead of one large abstract engine interface that reduces everything to the lowest common denominator, we take the **pragmatic middle ground**:

- Shared **utilities** in `internal/scripting`: conversion of `flow.Message` <-> engine env, buffer `[]int` <-> `[]byte` bridges
- **Per engine** a dedicated sub-package with an engine-specific implementation — not clamped together behind a common interface
- Each Function node has its own code path and uses the sub-package of its engine

```
internal/scripting/
  ├─ scripting.go          # MessageEnv, MessageFromEnv, BufferToInts, IntsToBuffer
  ├─ goja/
  │  └─ engine.go          # Goja wrapper (extracted from current function*.go)
  ├─ expr/
  │  └─ engine.go          # expr wrapper, compile-at-deploy
  └─ yaegi/
     └─ engine.go          # Yaegi wrapper, function reflection
```

Rationale: the three engines differ semantically too much for a common interface (Goja: function with side effects via `node.send`; expr: single expression with return value; Yaegi: Go function with reflected signature). A common interface would force all three to be dragged down to the Goja compromise.

### 2. Compile-at-deploy for all engines

Today the Goja Function node already compiles in `Start()` (see `internal/nodes/function.go:108-124`). That becomes the pattern for all three engines:

- **Init/Start of the node** -> engine-specific compile (`goja.Compile`, `expr.Compile`, `yaegi.Eval`)
- **Compile errors become deploy errors** — the node goes into status "red" with the error message
- **Per message** only run/call — no parsing/compile cost

Important UX consequence: typos in user code show up at deploy, not on the first message. With expr and Yaegi (statically typed), this also applies to type mismatches in the declared env.

### 3. Expr Function Node (`function-expr`)

#### Purpose

Pipeline-style transformations — `map`/`filter`/`reduce`/`sum` over lists, conditional object construction, simple aggregates. Idiomatically a single expression, no control flow, no mutations.

#### Configuration

- `expression` (string) — the expr expression. Operates on an env with:
  - `payload` — the `msg.payload` of the incoming message
  - `topic` — the `msg.topic`
  - `msg` — the complete message as a map (escape hatch for fields outside payload/topic)
- `outputProperty` (string, default `"payload"`) — which message field the result is written to
- `passThrough` (boolean, default `false`) — if `true`, the original message is preserved and only `outputProperty` is overwritten; if `false`, a new message with only the result + `topic` is emitted

#### Examples

```javascript
// Filter and aggregate over sensor readings
{
    avg: mean(map(payload, .temperature)),
    max: max(map(payload, .temperature)),
    above_threshold: count(filter(payload, .temperature > 25))
}
```

```javascript
// Conditional routing
filter(payload, .priority == "high")
```

```javascript
// Topic-based logic with pipe syntax
payload | map({ time: .timestamp, value: .temperature * 1.8 + 32 })
```

#### Properties panel

```
┌──────────────────────────────────────────────┐
│  Expr Function                                │
├──────────────────────────────────────────────┤
│                                               │
│  Expression                                   │
│  ┌────────────────────────────────────────┐   │
│  │ sum(filter(payload, .temp > 20))       │   │
│  │                                        │   │
│  │ // additional lines allowed            │   │
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

Compile errors are shown inline below the editor with line/column markers.

### 4. Go Function Node (`function-go`)

#### Purpose

Algorithms with real control flow (`for`, `if/else`, mutable state, multi-stage logic) that cannot be expressed as a single expr expression. Especially for **batch numeric** use cases that would be too slow in Goja.

#### Configuration

- `code` (string) — Go code with a function named `handle`
- `outputs` (number, default `1`) — number of output ports (analogous to the JS Function node)

#### Code convention

The user writes Go code with a function `handle` whose signature the engine reads via reflection:

```go
// Simplest form — payload as a generic value
func handle(payload any) any {
    // ...
    return payload
}
```

```go
// With typed map access
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
// With user-defined struct (typed path, fastest)
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

If the user picks typed structs, **the engine converts at the boundary** between `flow.Message` and the Yaegi function (via JSON marshal/unmarshal with the `json:` tags). That costs reflection — but is still faster than Goja map access on large batches, because the hot loop inside stays typed.

#### Buffer interop

Buffer payloads from mqtt-in arrive as `[]int`. The Yaegi code works **directly with `[]byte`**:

```go
import "encoding/binary"

func handle(payload []byte) int64 {
    return int64(binary.BigEndian.Uint64(payload[:8]))
}
```

The engine converts `[]int` -> `[]byte` on entry and back on exit. Via `internal/scripting.IntsToBuffer` and `BufferToInts`. The user notices nothing.

#### Multi-output via `node.send`

Analogous to the JS Function, the Yaegi code receives a `node` object:

```go
func handle(payload any, node Node) {
    if /* condition */ {
        node.Send(0, payload)
    } else {
        node.Send(1, payload)
    }
}
```

`Node` is exported into the Yaegi env as an interface, with methods `Send(port int, msg any)`, `Log(args ...any)`, `Warn(args ...any)`, `Error(args ...any)`, `Status(fill, text string)`.

#### Properties panel

Identical to the JS Function: a large code editor (with Go syntax highlighting) plus an `outputs` field. Inline compile errors with line markers.

### 5. Change Node — expr as a value type

The Change node today has rules with the following value types: `msg`, `flow`, `global`, `str`, `num`, `bool`, `json`, `date`, `env`. We add another one: **`expr`**.

#### Behavior

- Value type `expr` -> the value field content is an expr expression
- At deploy, all expr rules of a Change node are pre-compiled
- Per incoming message, the expression is evaluated against an env with `msg`/`flow`/`global`, the result is set on the target property

#### Example

| Property | Value Type | Value |
|---|---|---|
| `payload` | `expr` | `payload * 1.8 + 32` |
| `topic` | `expr` | `topic + "/converted"` |
| `severity` | `expr` | `payload.value > 50 ? "high" : "low"` |

#### UX

In the Change node properties panel, `expr` appears next to the existing value types in the dropdown. When selected, the value field becomes a smaller code editor (single-line or small textarea) with a syntax hint.

### 6. Naming and palette

All three Function nodes appear in the palette **separately**, with clearly distinguishable descriptions:

| Type ID | Label (palette) | Description |
|---|---|---|
| `function` | Function | JavaScript — logic and glue |
| `function-expr` | Expr Function | Single expression — fast pipelines, transforms |
| `function-go` | Go Function | Go code — fast batch processing |

**Deliberately no "default"**: the user picks by use case, no implicit "fast" / "slow" path.

## Data structure

### workspace.json — Function-Expr example

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

### workspace.json — Function-Go example

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

### workspace.json — Change node with expr rule

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

## Affected files

### Backend — new

- `internal/scripting/scripting.go` — shared utilities (`MessageEnv`, `MessageFromEnv`, `BufferToInts`, `IntsToBuffer`)
- `internal/scripting/goja/engine.go` — extracted Goja code (refactor of `internal/nodes/function*.go`)
- `internal/scripting/expr/engine.go` — expr wrapper with `Compile`/`Run` methods
- `internal/scripting/yaegi/engine.go` — Yaegi wrapper with reflection-based signature detection and boundary conversion
- `internal/nodes/function_expr.go` — Expr Function node
- `internal/nodes/function_go.go` — Go Function node
- `internal/nodes/function_expr_test.go` + `function_go_test.go` — tests per node
- `frontend/src/components/config/ExprFunctionConfig.vue`
- `frontend/src/components/config/GoFunctionConfig.vue`

### Backend — changes

- `internal/nodes/function.go` and `function_buffer.go` — refactored to use `internal/scripting/goja`, no UX change
- `internal/nodes/change.go` — new value type `expr`, compile loop in `Init()` for all expr rules
- `internal/server/server.go` — registration of `function-expr` and `function-go`

### Frontend — changes

- `frontend/src/components/config/ChangeNodeConfig.vue` (or whatever it is called) — `expr` as a value type option in the dropdown, code editor variant for the value field on `expr`
- `frontend/src/components/PropertyPanel.vue` — dispatch for the two new node types
- `frontend/src/components/nodes/tokens.ts` — palette tokens for `function-expr` (e.g. blue pipeline icon) and `function-go` (Go gopher or fast-forward)

### Go dependencies

- `github.com/expr-lang/expr` — already validated in the benchmark
- `github.com/traefik/yaegi` — already validated in the benchmark

## Technical notes

### Engine lifecycle per node

Both new engines follow the same pattern as the existing Goja Function node:

```go
// In Init(): only parse properties
func (n *ExprFunctionNode) Init() error {
    n.expression, _ = n.config.Properties["expression"].(string)
    // ... additional fields
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

// Per message: just run
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

### Yaegi — boundary conversion with typed structs

If the user function expects `[]Reading` as input but `flow.Message.Payload()` delivers a `[]map[string]any`, we need a conversion. Three options:

1. **JSON marshal/unmarshal** — pragmatic, ~100% reflection cost, but idiomatic via `json:` tags
2. **Reflection directly** — would put us in front of every field mapping, faster but complex
3. **Codegen** — build-time generation of a converter, fastest, but huge UX effort

We pick (1) — JSON marshal/unmarshal. Via `json:` tags the user has full control, the code is small. Performance: for 10k records with 3 fields, the conversion comes in at ~1-2ms. That is relevant but not prohibitive — the Yaegi hot loop in the typed path is still faster than Goja in the map path.

```go
// Engine side:
payload := msg.Payload()
inputType := reflect.TypeOf(handle).In(0)        // e.g. []Reading
typed := reflect.New(inputType).Interface()
data, _ := json.Marshal(payload)                 // []byte
json.Unmarshal(data, typed)                      // populated []Reading
result := callHandle(typed)                      // fast, typed
```

### Yaegi — code sandbox

Yaegi by default exposes the Go standard library via `i.Use(stdlib.Symbols)`. That gives user code access to `os.Open`, `net.Dial`, etc. — which we **do not want**.

We explicitly register only a **filtered subset**:
- `encoding/binary` — binary parsing
- `encoding/json` — JSON parsing
- `fmt` (only `Sprintf`, `Sprint`, no `Println`)
- `math`, `math/big`
- `strings`, `strconv`, `bytes`
- `sort`
- `time` (only parsing/formatting, no `Sleep` etc.)

Forbidden: `os`, `io/ioutil`, `net`, `net/http`, `runtime`, `unsafe`, `syscall`. If the user code imports those, the compile fails.

The list is maintained as a curated map in `internal/scripting/yaegi/symbols.go`.

### Expr — env shape and type validation

`expr.Compile` accepts an env shape:

```go
env := map[string]any{
    "payload": []any{},          // generic — "payload is a list"
    "topic":   "",
    "msg":     map[string]any{},
}
program, err := expr.Compile(userExpression, expr.Env(env))
```

That gives the compiler enough info to validate field access (`payload[0].x`), but leaves the user flexible. If the user wants typed strictness, they can give a hint via doc comment at the top of the expression — current version: simple `any`-based env, later extensible with an explicit-typed-shape property in the node.

### Change node — expr compile lifecycle

All rules with `valueType: "expr"` are pre-compiled in the Change node's `Init()`. Compile errors of a single rule turn the whole node into a deploy error (with clear localization which rule).

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

### Frontend — shared code editor

All three Function nodes (JS, expr, Go) plus the expr rules in the Change node use **the same code editor wrapper** (`SimpleEditor.vue` currently, on a Monaco basis). Difference: the language configuration:
- JS Function -> `language: 'javascript'`
- Go Function -> `language: 'go'`
- Expr Function -> `language: 'expr'` (custom Monaco tokenizer, simple — keywords + operators, no linter)

Compile errors are shown as inline decorations on the affected line marker. The engine returns position info on compile error; the Vue component passes that through to Monaco.

## Performance — what we promise

From the benchmark (`benchmark/bench_test.go`, AMD Ryzen 7 5800H, 1k to 100k records):

| Operation | Native Go | Goja (JS) | Expr | Yaegi (map) | Yaegi (typed) |
|---|---:|---:|---:|---:|---:|
| 1k records | 7.7 µs | 752 µs | 321 µs | 192 µs | 77 µs |
| 10k records | 80 µs | 6.87 ms | 3.16 ms | 1.77 ms | 773 µs |
| 100k records | 1.5 ms | 64.7 ms | 29.5 ms | 20.5 ms | 7.83 ms |
| **Speedup vs Goja @10k** | 86x | 1x | **2.2x** | **3.9x** | **8.9x** |

**Realism**: these are pure-computation numbers. In practice IO latencies, MQTT round-trips, workspace sync, etc. add up. The engine choice makes 0% difference in the IO-bound hot path; in the CPU-bound hot path the factor is as above.

## Dependencies

- No hard dependency on other issues
- Benefits from the v5 MQTT setup (`buffer` output mode delivers `[]int`, which Yaegi and Goja can interpret as `[]byte` — see Buffer interop)
- Assumes the config node concept from the MQTT issue (for any future engines that need config nodes themselves — not here, but good to know)

## Out of scope

- **WASM Function node** — a fourth engine via wazero for maximum performance on compute-heavy workloads. Useful, but a separate issue with its own UX (upload `.wasm`, no editor)
- **V8 bindings (cgo)** — contradicts the single-binary character of LOOPZE
- **Node.js subprocess** — gives up self-contained deployment
- **Reactive expressions** — expr programs that are re-evaluated on change of a source variable without a trigger message. Interesting for live UI bindings, but not the Function node model
- **Hot reload of user code without re-deploy** — would be an editor-experience boost but is technically complex (engine lifecycle, subscription re-wiring, state migration)
- **Multi-tenancy / strict sandboxing** — resource limits (CPU time, memory), secrets hiding etc. are addressed in the credential system, not here
- **Code sharing between Function nodes** — no `import` from one Function node into another. If needed: future "library node" concept
- **Auto engine choice** — no "magic Function node that picks Goja/expr/Yaegi based on input". The user picks explicitly
- **Type inference for Yaegi schema from sample messages** — would be nice but a separate UX effort
