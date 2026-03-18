<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, shallowRef } from 'vue'
import '@/lib/monaco/setup-workers'
import * as monaco from 'monaco-editor'
import { flintTypeDefinitions } from '@/lib/monaco/flint-types'

const props = withDefaults(defineProps<{
  modelValue: string
  readonly?: boolean
  placeholder?: string
  minHeight?: string
}>(), {
  modelValue: '',
  readonly: false,
  placeholder: '',
  minHeight: '180px',
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

/** Extract user code from the wrapped model content. */
function unwrap(fullContent: string): string {
  const lines = fullContent.split('\n')
  // Remove first line (wrapper prefix) and last line (wrapper suffix)
  return lines.slice(PREFIX_LINES, lines.length - 1).join('\n')
}

/** Build the full wrapped model content from user code. */
function wrap(userCode: string): string {
  return WRAPPER_PREFIX + userCode + WRAPPER_SUFFIX
}

onMounted(() => {
  if (!container.value) return

  configureMonaco()

  // Create a model with the user code wrapped in a function body.
  const uri = monaco.Uri.parse('file:///flint-function-' + Date.now() + '.js')
  const model = monaco.editor.createModel(wrap(props.modelValue), 'javascript', uri)

  editor.value = monaco.editor.create(container.value, {
    model,
    theme: 'flint-dark',
    readOnly: props.readonly,
    fontSize: 12,
    fontFamily: "'JetBrains Mono', 'Fira Mono', 'Consolas', monospace",
    fontLigatures: false,
    lineNumbers: (lineNumber) => String(lineNumber - PREFIX_LINES),
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

  // Hide the wrapper lines (first and last) from the user.
  const ed = editor.value;
  (ed as any).setHiddenAreas([
    // Hide the wrapper prefix line
    new monaco.Range(1, 1, PREFIX_LINES, 1),
    // Hide the wrapper suffix line
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
    internalUpdate = true;
    model.setValue(wrap(val));
    (ed as any).setHiddenAreas([
      new monaco.Range(1, 1, PREFIX_LINES, 1),
      new monaco.Range(model.getLineCount(), 1, model.getLineCount(), 1),
    ])
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
