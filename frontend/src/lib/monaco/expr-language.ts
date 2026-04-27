// Monarch tokenizer + completion provider for expr-lang
// (https://expr-lang.org). Registered once on first use. Keywords + builtins
// are based on the expr stdlib reference; the goal is readable highlighting
// and useful suggestions, not full validation (compile errors come from the
// backend on deploy).

import * as monaco from 'monaco-editor'

let registered = false

export function registerExprLanguage(): void {
  if (registered) return
  registered = true

  monaco.languages.register({ id: 'expr' })

  monaco.languages.setLanguageConfiguration('expr', {
    comments: { lineComment: '//', blockComment: ['/*', '*/'] },
    brackets: [
      ['{', '}'],
      ['[', ']'],
      ['(', ')'],
    ],
    autoClosingPairs: [
      { open: '{', close: '}' },
      { open: '[', close: ']' },
      { open: '(', close: ')' },
      { open: '"', close: '"', notIn: ['string'] },
      { open: "'", close: "'", notIn: ['string'] },
    ],
    surroundingPairs: [
      { open: '{', close: '}' },
      { open: '[', close: ']' },
      { open: '(', close: ')' },
      { open: '"', close: '"' },
      { open: "'", close: "'" },
    ],
  })

  monaco.languages.setMonarchTokensProvider('expr', {
    keywords: [
      'let', 'if', 'else', 'nil', 'true', 'false',
      'in', 'not', 'and', 'or',
      'matches', 'contains', 'startsWith', 'endsWith',
    ],
    builtins: [
      'all', 'any', 'one', 'none', 'filter', 'map', 'count',
      'sum', 'mean', 'median', 'min', 'max', 'reduce',
      'len', 'keys', 'values', 'take', 'first', 'last',
      'groupBy', 'sortBy', 'reverse', 'flatten',
      'string', 'int', 'float', 'type',
      'now', 'duration', 'date',
      'abs', 'ceil', 'floor', 'round',
      'lower', 'upper', 'trim', 'split', 'join', 'replace', 'repeat',
      'toJSON', 'fromJSON', 'toBase64', 'fromBase64',
    ],
    operators: [
      '==', '!=', '<', '<=', '>', '>=',
      '+', '-', '*', '/', '%', '**',
      '?', ':', '|', '??', '?.', '..',
    ],
    symbols: /[=><!~?:&|+\-*/^%]+/,
    tokenizer: {
      root: [
        [/[a-zA-Z_$][\w$]*/, {
          cases: {
            '@keywords': 'keyword',
            '@builtins': 'function',
            '@default': 'identifier',
          },
        }],
        // pipe + dot accessor chains for the `|` and `.field` syntax
        [/\.\w+/, 'property'],
        [/[{}()[\]]/, '@brackets'],
        [/@symbols/, {
          cases: {
            '@operators': 'operator',
            '@default': '',
          },
        }],
        // numbers
        [/\d*\.\d+([eE][\-+]?\d+)?/, 'number.float'],
        [/0[xX][0-9a-fA-F]+/, 'number.hex'],
        [/\d+/, 'number'],
        // strings
        [/"([^"\\]|\\.)*$/, 'string.invalid'],
        [/"/, { token: 'string.quote', bracket: '@open', next: '@stringDouble' }],
        [/'([^'\\]|\\.)*$/, 'string.invalid'],
        [/'/, { token: 'string.quote', bracket: '@open', next: '@stringSingle' }],
        // comments
        [/\/\/.*$/, 'comment'],
        [/\/\*/, 'comment', '@comment'],
        // whitespace
        [/[ \t\r\n]+/, ''],
      ],
      stringDouble: [
        [/[^\\"]+/, 'string'],
        [/\\./, 'string.escape'],
        [/"/, { token: 'string.quote', bracket: '@close', next: '@pop' }],
      ],
      stringSingle: [
        [/[^\\']+/, 'string'],
        [/\\./, 'string.escape'],
        [/'/, { token: 'string.quote', bracket: '@close', next: '@pop' }],
      ],
      comment: [
        [/[^/*]+/, 'comment'],
        [/\*\//, 'comment', '@pop'],
        [/[/*]/, 'comment'],
      ],
    },
  })

  registerCompletions()
}

// Per-symbol completion metadata. Snippets use ${1:placeholder} per Monaco's
// snippet syntax; we wrap callable builtins so a tab-stop lands inside the
// parentheses when the user accepts the suggestion.
interface BuiltinDoc {
  name: string
  snippet: string
  detail: string
  doc: string
}

const VARIABLES = [
  { name: 'payload', detail: 'msg.payload — incoming value' },
  { name: 'topic',   detail: 'msg.topic — incoming topic (string)' },
  { name: 'msg',     detail: 'full message map (escape-hatch for less-common fields)' },
] as const

const BUILTINS: BuiltinDoc[] = [
  // Pipeline / collection
  { name: 'map',       snippet: 'map(${1:list}, ${2:.field})',                detail: 'map(list, predicate)',         doc: 'Transform each element. Predicate uses the .field shorthand.' },
  { name: 'filter',    snippet: 'filter(${1:list}, ${2:.field > 0})',         detail: 'filter(list, predicate)',      doc: 'Keep only matching elements.' },
  { name: 'reduce',    snippet: 'reduce(${1:list}, ${2:#acc + #}, ${3:0})',   detail: 'reduce(list, fn, init)',       doc: 'Fold a list. Use # for the current element, #acc for the accumulator.' },
  { name: 'all',       snippet: 'all(${1:list}, ${2:.field})',                detail: 'all(list, predicate) bool',    doc: 'True if predicate holds for every element.' },
  { name: 'any',       snippet: 'any(${1:list}, ${2:.field})',                detail: 'any(list, predicate) bool',    doc: 'True if predicate holds for any element.' },
  { name: 'one',       snippet: 'one(${1:list}, ${2:.field})',                detail: 'one(list, predicate) bool',    doc: 'True if exactly one element matches.' },
  { name: 'none',      snippet: 'none(${1:list}, ${2:.field})',               detail: 'none(list, predicate) bool',   doc: 'True if no element matches.' },
  { name: 'count',     snippet: 'count(${1:list}, ${2:.field})',              detail: 'count(list, predicate?) int',  doc: 'Count matching elements; without predicate counts the list.' },

  // Aggregates
  { name: 'sum',       snippet: 'sum(${1:list})',     detail: 'sum(list) number',     doc: 'Sum of numbers in the list.' },
  { name: 'mean',      snippet: 'mean(${1:list})',    detail: 'mean(list) number',    doc: 'Arithmetic mean.' },
  { name: 'median',    snippet: 'median(${1:list})',  detail: 'median(list) number',  doc: 'Median value.' },
  { name: 'min',       snippet: 'min(${1:list})',     detail: 'min(list) any',        doc: 'Minimum value.' },
  { name: 'max',       snippet: 'max(${1:list})',     detail: 'max(list) any',        doc: 'Maximum value.' },
  { name: 'len',       snippet: 'len(${1:value})',    detail: 'len(value) int',       doc: 'Length of a string, list, or map.' },

  // Collection helpers
  { name: 'keys',      snippet: 'keys(${1:map})',                              detail: 'keys(map) list',         doc: 'List of keys.' },
  { name: 'values',    snippet: 'values(${1:map})',                            detail: 'values(map) list',       doc: 'List of values.' },
  { name: 'take',      snippet: 'take(${1:list}, ${2:n})',                     detail: 'take(list, n) list',     doc: 'First n elements.' },
  { name: 'first',     snippet: 'first(${1:list})',                            detail: 'first(list) any',        doc: 'First element (nil if empty).' },
  { name: 'last',      snippet: 'last(${1:list})',                             detail: 'last(list) any',         doc: 'Last element (nil if empty).' },
  { name: 'groupBy',   snippet: 'groupBy(${1:list}, ${2:.field})',             detail: 'groupBy(list, key) map', doc: 'Group elements by key.' },
  { name: 'sortBy',    snippet: 'sortBy(${1:list}, ${2:.field})',              detail: 'sortBy(list, key) list', doc: 'Sort by extracted key.' },
  { name: 'reverse',   snippet: 'reverse(${1:list})',                          detail: 'reverse(list) list',     doc: 'Reverse order.' },
  { name: 'flatten',   snippet: 'flatten(${1:list})',                          detail: 'flatten(list) list',     doc: 'Flatten nested lists by one level.' },

  // Type & math
  { name: 'string',    snippet: 'string(${1:value})',                          detail: 'string(value) string', doc: 'Coerce to string.' },
  { name: 'int',       snippet: 'int(${1:value})',                             detail: 'int(value) int',       doc: 'Coerce to int.' },
  { name: 'float',     snippet: 'float(${1:value})',                           detail: 'float(value) float',   doc: 'Coerce to float.' },
  { name: 'type',      snippet: 'type(${1:value})',                            detail: 'type(value) string',   doc: 'Type name as string.' },
  { name: 'abs',       snippet: 'abs(${1:n})',                                 detail: 'abs(n) number',        doc: 'Absolute value.' },
  { name: 'ceil',      snippet: 'ceil(${1:n})',                                detail: 'ceil(n) number',       doc: 'Round up.' },
  { name: 'floor',     snippet: 'floor(${1:n})',                               detail: 'floor(n) number',      doc: 'Round down.' },
  { name: 'round',     snippet: 'round(${1:n})',                               detail: 'round(n) number',      doc: 'Round to nearest integer.' },

  // Strings
  { name: 'lower',     snippet: 'lower(${1:s})',                               detail: 'lower(s) string',                 doc: 'Lowercase.' },
  { name: 'upper',     snippet: 'upper(${1:s})',                               detail: 'upper(s) string',                 doc: 'Uppercase.' },
  { name: 'trim',      snippet: 'trim(${1:s})',                                detail: 'trim(s) string',                  doc: 'Strip leading/trailing whitespace.' },
  { name: 'split',     snippet: 'split(${1:s}, ${2:","})',                     detail: 'split(s, sep) list',              doc: 'Split a string into parts.' },
  { name: 'join',      snippet: 'join(${1:list}, ${2:","})',                   detail: 'join(list, sep) string',          doc: 'Join elements with separator.' },
  { name: 'replace',   snippet: 'replace(${1:s}, ${2:"old"}, ${3:"new"})',     detail: 'replace(s, old, new) string',     doc: 'Replace all occurrences.' },
  { name: 'repeat',    snippet: 'repeat(${1:s}, ${2:n})',                      detail: 'repeat(s, n) string',             doc: 'Repeat n times.' },

  // Encoding & time
  { name: 'toJSON',    snippet: 'toJSON(${1:value})',                          detail: 'toJSON(value) string', doc: 'Encode as JSON.' },
  { name: 'fromJSON',  snippet: 'fromJSON(${1:s})',                            detail: 'fromJSON(s) any',      doc: 'Decode JSON string.' },
  { name: 'toBase64',  snippet: 'toBase64(${1:value})',                        detail: 'toBase64(value) string', doc: 'Encode as base64.' },
  { name: 'fromBase64',snippet: 'fromBase64(${1:s})',                          detail: 'fromBase64(s) any',    doc: 'Decode base64 string.' },
  { name: 'now',       snippet: 'now()',                                       detail: 'now() time',           doc: 'Current time.' },
  { name: 'duration',  snippet: 'duration(${1:"5m"})',                         detail: 'duration(s) duration', doc: 'Parse a duration string ("5m", "1h30m").' },
  { name: 'date',      snippet: 'date(${1:"2026-01-01"})',                     detail: 'date(s) time',         doc: 'Parse a date string.' },

  // String operators that are binary keywords (also expose as built-ins for autocomplete)
  { name: 'matches',   snippet: '${1:s} matches "${2:pattern}"',               detail: 'matches  (regex)', doc: 'Regex match.' },
  { name: 'contains',  snippet: '${1:s} contains "${2:substr}"',               detail: 'contains',         doc: 'Substring containment.' },
  { name: 'startsWith',snippet: '${1:s} startsWith "${2:prefix}"',             detail: 'startsWith',       doc: 'Prefix check.' },
  { name: 'endsWith',  snippet: '${1:s} endsWith "${2:suffix}"',               detail: 'endsWith',         doc: 'Suffix check.' },
]

const KEYWORDS = ['let', 'if', 'else', 'nil', 'true', 'false', 'in', 'not', 'and', 'or']

function registerCompletions(): void {
  monaco.languages.registerCompletionItemProvider('expr', {
    triggerCharacters: ['.', '(', ' '],
    provideCompletionItems(model, position) {
      const word = model.getWordUntilPosition(position)
      const range: monaco.IRange = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn,
      }

      const suggestions: monaco.languages.CompletionItem[] = []

      for (const v of VARIABLES) {
        suggestions.push({
          label: v.name,
          kind: monaco.languages.CompletionItemKind.Variable,
          detail: v.detail,
          insertText: v.name,
          range,
        })
      }

      for (const b of BUILTINS) {
        suggestions.push({
          label: b.name,
          kind: monaco.languages.CompletionItemKind.Function,
          detail: b.detail,
          documentation: { value: b.doc },
          insertText: b.snippet,
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range,
        })
      }

      for (const kw of KEYWORDS) {
        suggestions.push({
          label: kw,
          kind: monaco.languages.CompletionItemKind.Keyword,
          insertText: kw,
          range,
        })
      }

      return { suggestions }
    },
  })
}
