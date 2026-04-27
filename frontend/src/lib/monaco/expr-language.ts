// Monarch tokenizer for expr-lang (https://expr-lang.org).
// Registered once on first use. Keywords + builtins are based on the expr
// stdlib reference; the goal is readable highlighting, not full validation
// (compile errors come from the backend on deploy).

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
}
