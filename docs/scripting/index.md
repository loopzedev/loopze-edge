# Scripting

LOOPZE ships two scripting engines side by side:

- **JavaScript** via [Goja](https://github.com/dop251/goja) — for complex
  logic, multiple statements, helpers, branching.
- **expr-lang** — for fast, single-expression checks (e.g. Switch rule
  conditions).

!!! note "TODO"
    Detailed reference, examples and gotchas to be filled in.

## When to use which

| Use case                                  | Engine     |
|-------------------------------------------|------------|
| Mutating `msg`, branching, helper funcs   | JavaScript |
| `payload > 50 && topic startsWith "hvac"` | expr-lang  |
| Rendering a string from `msg`             | Template node (Mustache / Go) |

## JavaScript runtime

- Single-expression *and* multi-statement scripts.
- Access to `msg`, `node`, `flow`, `global`, `env`.
- No Node.js APIs (no `fs`, no `require`). LOOPZE is a Go runtime, not V8.

## expr-lang

A fast, sandboxed expression language. Returns a single value. Used in
node configuration fields where a full script would be overkill — for
example Switch rules and Inject payload templates.

See [expr-lang.org](https://expr-lang.org/) for the language reference.
