<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, shallowRef } from 'vue'
import '@/lib/monaco/setup-workers'
import * as monaco from 'monaco-editor'
import { flintTypeDefinitions } from '@/lib/monaco/flint-types'
import { registerExprLanguage } from '@/lib/monaco/expr-language'

const props = withDefaults(defineProps<{
  modelValue: string
  readonly?: boolean
  placeholder?: string
  minHeight?: string
  language?: 'javascript' | 'plaintext' | 'expr' | 'go'
}>(), {
  modelValue: '',
  readonly: false,
  placeholder: '',
  minHeight: '180px',
  language: 'javascript',
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const container = ref<HTMLElement | null>(null)
const editor = shallowRef<monaco.editor.IStandaloneCodeEditor | null>(null)
let internalUpdate = false

// The wrapper prefix/suffix that the Go backend uses.
// We prepend this to the model so Monaco's TS service
// sees the code inside a function body (making `return` valid).
const WRAPPER_PREFIX = '(function(msg, node, global, flow, console) {\n'
const WRAPPER_SUFFIX = '\n})(msg, node, global, flow, console);'
const PREFIX_LINES = WRAPPER_PREFIX.split('\n').length - 1 // 1

// ── Configure Monaco theme & defaults once ───────────────────────
let configured = false
function configureMonaco() {
  if (configured) return
  configured = true

  monaco.editor.defineTheme('flint-dark', {
    base: 'vs-dark',
    inherit: true,
    rules: [
      { token: 'comment', foreground: '7d8590', fontStyle: 'italic' },
      { token: 'keyword', foreground: 'ff7b72' },
      { token: 'string', foreground: 'a5d6ff' },
      { token: 'number', foreground: '79c0ff' },
      { token: 'regexp', foreground: 'a5d6ff' },
      { token: 'type', foreground: 'ffa657' },
      { token: 'class', foreground: 'ffa657' },
      { token: 'function', foreground: 'd2a8ff' },
      { token: 'variable', foreground: 'e6edf3' },
      { token: 'constant', foreground: '79c0ff' },
      { token: 'parameter', foreground: 'e6edf3' },
      { token: 'property', foreground: '79c0ff' },
      { token: 'operator', foreground: 'ff7b72' },
      { token: 'delimiter', foreground: 'e6edf3' },
      { token: 'delimiter.bracket', foreground: 'e6edf3' },
    ],
    colors: {
      'editor.background': '#0d1117',
      'editor.foreground': '#e6edf3',
      'editor.lineHighlightBackground': '#161b2240',
      'editor.selectionBackground': '#58a6ff33',
      'editor.inactiveSelectionBackground': '#58a6ff1a',
      'editorCursor.foreground': '#58a6ff',
      'editorLineNumber.foreground': '#484f58',
      'editorLineNumber.activeForeground': '#7d8590',
      'editorGutter.background': '#0d1117',
      'editorWidget.background': '#161b22',
      'editorWidget.border': '#30363d',
      'editorSuggestWidget.background': '#161b22',
      'editorSuggestWidget.border': '#30363d',
      'editorSuggestWidget.selectedBackground': '#58a6ff33',
      'editorSuggestWidget.highlightForeground': '#58a6ff',
      'editorHoverWidget.background': '#161b22',
      'editorHoverWidget.border': '#30363d',
      'input.background': '#0d1117',
      'input.border': '#30363d',
      'input.foreground': '#e6edf3',
      'focusBorder': '#58a6ff',
      'list.hoverBackground': '#161b22',
      'list.activeSelectionBackground': '#58a6ff33',
      'scrollbarSlider.background': '#484f5833',
      'scrollbarSlider.hoverBackground': '#484f5866',
      'scrollbarSlider.activeBackground': '#484f58aa',
      'editorBracketMatch.background': '#58a6ff33',
      'editorBracketMatch.border': '#58a6ff66',
      'editorIndentGuide.background': '#21262d',
      'editorIndentGuide.activeBackground': '#30363d',
    },
  })

  // Configure JS/TS defaults
  const ts = (monaco.languages as any).typescript
  if (ts) {
    ts.javascriptDefaults.setDiagnosticsOptions({
      noSemanticValidation: false,
      noSyntaxValidation: false,
    })

    ts.javascriptDefaults.setCompilerOptions({
      target: ts.ScriptTarget.ES2020,
      allowNonTsExtensions: true,
      allowJs: true,
      checkJs: true,
      strict: false,
      noEmit: true,
      lib: ['es2020'],
    })

    // Add Flint API type definitions
    ts.javascriptDefaults.addExtraLib(
      flintTypeDefinitions,
      'ts:flint-runtime.d.ts'
    )
  }
}

const isJS = props.language === 'javascript'

// File-extension hint for the Monaco model URI. Each language gets a unique
// extension so Monaco picks the right tokenizer; `.txt` falls back to plain.
const FILE_EXT: Record<string, string> = {
  javascript: 'js',
  go: 'go',
  expr: 'expr',
  plaintext: 'txt',
}

/** Extract user code from the wrapped model content. */
function unwrap(fullContent: string): string {
  if (!isJS) return fullContent
  const lines = fullContent.split('\n')
  // Remove first line (wrapper prefix) and last line (wrapper suffix)
  return lines.slice(PREFIX_LINES, lines.length - 1).join('\n')
}

/** Build the full wrapped model content from user code. */
function wrap(userCode: string): string {
  if (!isJS) return userCode
  return WRAPPER_PREFIX + userCode + WRAPPER_SUFFIX
}

onMounted(() => {
  if (!container.value) return

  configureMonaco()
  if (props.language === 'expr') {
    registerExprLanguage()
  }

  // Create a model. JS gets wrapped in a function body so Monaco's TS service
  // sees `return` as valid. Other languages are used as-is.
  const ext = FILE_EXT[props.language] ?? 'txt'
  const uri = monaco.Uri.parse(
    `file:///flint-${props.language}-${Date.now()}.${ext}`,
  )
  const model = monaco.editor.createModel(wrap(props.modelValue), props.language, uri)

  editor.value = monaco.editor.create(container.value, {
    model,
    theme: 'flint-dark',
    readOnly: props.readonly,
    fontSize: 12,
    fontFamily: "'IBM Plex Mono', monospace",
    fontLigatures: false,
    lineNumbers: isJS ? (lineNumber) => String(lineNumber - PREFIX_LINES) : 'on',
    lineNumbersMinChars: 3,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    automaticLayout: true,
    tabSize: 2,
    insertSpaces: true,
    wordWrap: 'on',
    wrappingIndent: 'indent',
    bracketPairColorization: { enabled: true },
    matchBrackets: 'always',
    autoClosingBrackets: 'always',
    autoClosingQuotes: 'always',
    autoIndent: 'full',
    formatOnPaste: true,
    suggestOnTriggerCharacters: true,
    quickSuggestions: {
      other: true,
      comments: false,
      strings: false,
    },
    parameterHints: { enabled: true },
    hover: { enabled: true },
    folding: true,
    renderLineHighlight: 'line',
    overviewRulerBorder: false,
    overviewRulerLanes: 0,
    hideCursorInOverviewRuler: true,
    contextmenu: false,
    scrollbar: {
      verticalScrollbarSize: 8,
      horizontalScrollbarSize: 8,
      useShadows: false,
    },
    padding: { top: 8, bottom: 8 },
    fixedOverflowWidgets: true,
  })

  const ed = editor.value

  if (isJS) {
    // Hide the wrapper lines (first and last) from the user.
    (ed as any).setHiddenAreas([
      new monaco.Range(1, 1, PREFIX_LINES, 1),
      new monaco.Range(model.getLineCount(), 1, model.getLineCount(), 1),
    ])

    // Place cursor at the start of user code
    ed.setPosition({ lineNumber: PREFIX_LINES + 1, column: 1 })

    // Prevent editing the hidden wrapper lines
    ed.onDidChangeModelContent((e) => {
      if (internalUpdate) return

      // Check if any change touched the wrapper lines
      for (const change of e.changes) {
        if (change.range.startLineNumber <= PREFIX_LINES ||
            change.range.startLineNumber >= model.getLineCount()) {
          // Undo wrapper modifications
          internalUpdate = true
          ed.trigger('flint', 'undo', null)
          internalUpdate = false
          return
        }
      }

      internalUpdate = true
      emit('update:modelValue', unwrap(model.getValue()));
      internalUpdate = false;

      // Re-hide wrapper suffix (line count may have changed)
      (ed as any).setHiddenAreas([
        new monaco.Range(1, 1, PREFIX_LINES, 1),
        new monaco.Range(model.getLineCount(), 1, model.getLineCount(), 1),
      ])
    })
  } else {
    ed.onDidChangeModelContent(() => {
      if (internalUpdate) return
      internalUpdate = true
      emit('update:modelValue', model.getValue())
      internalUpdate = false
    })
  }
})

onUnmounted(() => {
  const ed = editor.value
  if (ed) {
    ed.getModel()?.dispose()
    ed.dispose()
  }
  editor.value = null
})

// Sync external model changes
watch(() => props.modelValue, (val) => {
  if (internalUpdate) return
  const ed = editor.value
  if (!ed) return
  const model = ed.getModel()
  if (!model) return

  const current = unwrap(model.getValue())
  if (val !== current) {
    internalUpdate = true
    model.setValue(wrap(val))
    if (isJS) {
      (ed as any).setHiddenAreas([
        new monaco.Range(1, 1, PREFIX_LINES, 1),
        new monaco.Range(model.getLineCount(), 1, model.getLineCount(), 1),
      ])
    }
    internalUpdate = false
  }
})
</script>

<template>
  <div
    ref="container"
    class="w-full overflow-hidden border border-terminal-border rounded-sm flex-1"
    :style="{ minHeight: minHeight }"
    @keydown.stop
    @keyup.stop
    @keypress.stop
  />
</template>
