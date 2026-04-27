// Flint-specific completion provider for Go (function-go node).
//
// Monaco ships a Go tokenizer for syntax highlighting but no language service,
// so user code in a function-go node has no IntelliSense out of the box. This
// module adds completions for:
//
//   - the supported `handle` signatures (snippets)
//   - flintnode.Node interface methods (after `node.`)
//   - allowed stdlib imports (after `import "`)
//
// Registered once on first use via registerGoCompletions(). Compile errors
// still come from the backend on deploy — these are completions, not validation.

import * as monaco from 'monaco-editor'

let registered = false

interface MethodDoc {
  name: string
  signature: string
  insertText: string
  detail: string
  doc: string
}

// Methods on the flintnode.Node interface — must stay in sync with
// internal/scripting/yaegi/engine.go.
const NODE_METHODS: MethodDoc[] = [
  { name: 'Send',         signature: 'Send(port int, msg any)',         insertText: 'Send(${1:0}, ${2:payload})',           detail: 'Send(port int, msg any)',                doc: 'Emit msg on output port. Use for multi-output.' },
  { name: 'Log',          signature: 'Log(args ...any)',                insertText: 'Log(${1:args})',                       detail: 'Log(args ...any)',                       doc: 'Emit a debug message (status "debug").' },
  { name: 'Warn',         signature: 'Warn(args ...any)',               insertText: 'Warn(${1:args})',                      detail: 'Warn(args ...any)',                      doc: 'Emit a debug message (status "warn").' },
  { name: 'Error',        signature: 'Error(args ...any)',              insertText: 'Error(${1:args})',                     detail: 'Error(args ...any)',                     doc: 'Emit a debug message (status "error").' },
  { name: 'Status',       signature: 'Status(fill, text string)',       insertText: 'Status("${1:green}", "${2:ok}")',      detail: 'Status(fill, text string)',              doc: 'Set the node\'s status indicator. fill: green/red/yellow/blue/grey.' },

  // Node-scoped in-memory context
  { name: 'Get',          signature: 'Get(key string) any',             insertText: 'Get("${1:key}")',                      detail: 'Get(key) any',                           doc: 'Read from node-scoped in-memory context.' },
  { name: 'Set',          signature: 'Set(key string, val any)',        insertText: 'Set("${1:key}", ${2:value})',          detail: 'Set(key, val)',                          doc: 'Write to node-scoped in-memory context.' },
  { name: 'Delete',       signature: 'Delete(key string)',              insertText: 'Delete("${1:key}")',                   detail: 'Delete(key)',                            doc: 'Remove from node-scoped in-memory context.' },

  // Flow-scoped context
  { name: 'FlowGet',      signature: 'FlowGet(key string) any',         insertText: 'FlowGet("${1:key}")',                  detail: 'FlowGet(key) any',                       doc: 'Read from flow-scoped memory store.' },
  { name: 'FlowSet',      signature: 'FlowSet(key string, val any)',    insertText: 'FlowSet("${1:key}", ${2:value})',      detail: 'FlowSet(key, val)',                      doc: 'Write to flow-scoped memory store.' },
  { name: 'FlowDelete',   signature: 'FlowDelete(key string)',          insertText: 'FlowDelete("${1:key}")',               detail: 'FlowDelete(key)',                        doc: 'Remove from flow-scoped memory store.' },
  { name: 'FlowGetP',     signature: 'FlowGetP(key string) any',        insertText: 'FlowGetP("${1:key}")',                 detail: 'FlowGetP(key) any',                      doc: 'Read from flow-scoped persistent (file-backed) store.' },
  { name: 'FlowSetP',     signature: 'FlowSetP(key string, val any)',   insertText: 'FlowSetP("${1:key}", ${2:value})',     detail: 'FlowSetP(key, val)',                     doc: 'Write to flow-scoped persistent store.' },
  { name: 'FlowDeleteP',  signature: 'FlowDeleteP(key string)',         insertText: 'FlowDeleteP("${1:key}")',              detail: 'FlowDeleteP(key)',                       doc: 'Remove from flow-scoped persistent store.' },

  // Global context
  { name: 'GlobalGet',     signature: 'GlobalGet(key string) any',       insertText: 'GlobalGet("${1:key}")',                detail: 'GlobalGet(key) any',                    doc: 'Read from global memory store.' },
  { name: 'GlobalSet',     signature: 'GlobalSet(key string, val any)',  insertText: 'GlobalSet("${1:key}", ${2:value})',    detail: 'GlobalSet(key, val)',                   doc: 'Write to global memory store.' },
  { name: 'GlobalDelete',  signature: 'GlobalDelete(key string)',        insertText: 'GlobalDelete("${1:key}")',             detail: 'GlobalDelete(key)',                     doc: 'Remove from global memory store.' },
  { name: 'GlobalGetP',    signature: 'GlobalGetP(key string) any',      insertText: 'GlobalGetP("${1:key}")',               detail: 'GlobalGetP(key) any',                   doc: 'Read from global persistent store.' },
  { name: 'GlobalSetP',    signature: 'GlobalSetP(key string, val any)', insertText: 'GlobalSetP("${1:key}", ${2:value})',   detail: 'GlobalSetP(key, val)',                  doc: 'Write to global persistent store.' },
  { name: 'GlobalDeleteP', signature: 'GlobalDeleteP(key string)',       insertText: 'GlobalDeleteP("${1:key}")',            detail: 'GlobalDeleteP(key)',                    doc: 'Remove from global persistent store.' },
]

// Allowed stdlib imports — must stay in sync with
// internal/scripting/yaegi/symbols.go (allowedPackages).
const ALLOWED_IMPORTS = [
  'bytes',
  'encoding/base64',
  'encoding/binary',
  'encoding/hex',
  'encoding/json',
  'errors',
  'flintnode',
  'fmt',
  'math',
  'math/big',
  'math/bits',
  'math/rand',
  'regexp',
  'sort',
  'strconv',
  'strings',
  'time',
  'unicode',
  'unicode/utf8',
  'unicode/utf16',
]

// Top-level handle templates — surface common signatures as snippets so the
// user can scaffold the function quickly. Triggered from any top-level word
// completion.
const HANDLE_TEMPLATES = [
  {
    label: 'handle (any → any)',
    detail: 'pass-through with editable body',
    insertText: [
      'func handle(payload any) any {',
      '\t${1:return payload}',
      '}',
    ].join('\n'),
  },
  {
    label: 'handle (map slice)',
    detail: 'iterate []map[string]any',
    insertText: [
      'func handle(payload []map[string]any) []map[string]any {',
      '\tout := []map[string]any{}',
      '\tfor _, r := range payload {',
      '\t\tif ${1:cond} {',
      '\t\t\tout = append(out, r)',
      '\t\t}',
      '\t}',
      '\treturn out',
      '}',
    ].join('\n'),
  },
  {
    label: 'handle (typed struct)',
    detail: 'JSON-tagged struct with handle',
    insertText: [
      'type ${1:Reading} struct {',
      '\t${2:Temperature} float64 `json:"${3:temperature}"`',
      '}',
      '',
      'func handle(payload []${1:Reading}) []${1:Reading} {',
      '\tout := []${1:Reading}{}',
      '\tfor _, r := range payload {',
      '\t\tif r.${2:Temperature} > ${4:0} {',
      '\t\t\tout = append(out, r)',
      '\t\t}',
      '\t}',
      '\treturn out',
      '}',
    ].join('\n'),
  },
  {
    label: 'handle (with node — multi-output)',
    detail: 'route via flintnode.Node.Send',
    insertText: [
      'func handle(payload any, node flintnode.Node) {',
      '\tif ${1:cond} {',
      '\t\tnode.Send(0, payload)',
      '\t} else {',
      '\t\tnode.Send(1, payload)',
      '\t}',
      '}',
    ].join('\n'),
  },
  {
    label: 'handle (buffer parse)',
    detail: '[]byte input + binary parsing',
    insertText: [
      'func handle(payload []byte) ${1:uint64} {',
      '\treturn binary.BigEndian.${2:Uint64}(payload[:${3:8}])',
      '}',
    ].join('\n'),
  },
]

export function registerGoCompletions(): void {
  if (registered) return
  registered = true

  monaco.languages.registerCompletionItemProvider('go', {
    triggerCharacters: ['.', '"'],
    provideCompletionItems(model, position) {
      const word = model.getWordUntilPosition(position)
      const range: monaco.IRange = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn,
      }

      // Read the line up to the cursor so we can be context-aware about
      // what the user is typing — node.<X>, import "<X>, etc.
      const lineUntilPos = model.getValueInRange({
        startLineNumber: position.lineNumber,
        startColumn: 1,
        endLineNumber: position.lineNumber,
        endColumn: position.column,
      })

      // After "node." → flintnode.Node interface methods
      if (/\bnode\.[\w]*$/.test(lineUntilPos)) {
        return {
          suggestions: NODE_METHODS.map(m => ({
            label: m.name,
            kind: monaco.languages.CompletionItemKind.Method,
            detail: m.detail,
            documentation: { value: m.doc },
            insertText: m.insertText,
            insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
            range,
          })),
        }
      }

      // After `import "` → allowed stdlib paths
      if (/\bimport\s*(?:\(\s*)?"[^"]*$/.test(lineUntilPos)) {
        // Drop the closing quote since the user's cursor is between the quotes.
        return {
          suggestions: ALLOWED_IMPORTS.map(pkg => ({
            label: pkg,
            kind: monaco.languages.CompletionItemKind.Module,
            detail: `import "${pkg}"`,
            insertText: pkg,
            range,
          })),
        }
      }

      // Top-level: handle-function snippets + a few keywords/builtins
      const suggestions: monaco.languages.CompletionItem[] = []

      for (const t of HANDLE_TEMPLATES) {
        suggestions.push({
          label: t.label,
          kind: monaco.languages.CompletionItemKind.Snippet,
          detail: t.detail,
          insertText: t.insertText,
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range,
        })
      }

      // Bare "node" identifier — useful when the user starts typing in a
      // handle that has the second argument.
      suggestions.push({
        label: 'node',
        kind: monaco.languages.CompletionItemKind.Variable,
        detail: 'flintnode.Node — runtime API',
        insertText: 'node',
        range,
      })

      // Frequent imports as quick-insert snippets at the top of the file.
      suggestions.push({
        label: 'import "encoding/binary"',
        kind: monaco.languages.CompletionItemKind.Snippet,
        detail: 'binary parsing',
        insertText: 'import "encoding/binary"',
        range,
      })
      suggestions.push({
        label: 'import "encoding/json"',
        kind: monaco.languages.CompletionItemKind.Snippet,
        detail: 'JSON encoding/decoding',
        insertText: 'import "encoding/json"',
        range,
      })
      suggestions.push({
        label: 'import "flintnode"',
        kind: monaco.languages.CompletionItemKind.Snippet,
        detail: 'Flint runtime API (node.Send, node.Log, …)',
        insertText: 'import "flintnode"',
        range,
      })

      return { suggestions }
    },
  })
}
