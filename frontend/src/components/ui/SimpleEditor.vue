<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, shallowRef } from 'vue'
import '@/lib/monaco/setup-workers'
import * as monaco from 'monaco-editor'

const props = withDefaults(defineProps<{
  modelValue: string
  language?: 'json' | 'javascript'
  readonly?: boolean
  placeholder?: string
}>(), {
  modelValue: '',
  language: 'json',
  readonly: false,
  placeholder: '',
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const container = ref<HTMLElement | null>(null)
const editor = shallowRef<monaco.editor.IStandaloneCodeEditor | null>(null)
let internalUpdate = false

// Reuse flint-dark theme if already defined, otherwise define it.
let themeConfigured = false
function ensureTheme() {
  if (themeConfigured) return
  themeConfigured = true
  try {
    monaco.editor.defineTheme('flint-dark', {
      base: 'vs-dark',
      inherit: true,
      rules: [
        { token: 'comment', foreground: '7d8590', fontStyle: 'italic' },
        { token: 'keyword', foreground: 'ff7b72' },
        { token: 'string', foreground: 'a5d6ff' },
        { token: 'number', foreground: '79c0ff' },
        { token: 'type', foreground: 'ffa657' },
        { token: 'function', foreground: 'd2a8ff' },
        { token: 'variable', foreground: 'e6edf3' },
        { token: 'property', foreground: '79c0ff' },
        { token: 'operator', foreground: 'ff7b72' },
        { token: 'delimiter', foreground: 'e6edf3' },
      ],
      colors: {
        'editor.background': '#0d1117',
        'editor.foreground': '#e6edf3',
        'editor.lineHighlightBackground': '#161b2240',
        'editor.selectionBackground': '#58a6ff33',
        'editorCursor.foreground': '#58a6ff',
        'editorLineNumber.foreground': '#484f58',
        'editorLineNumber.activeForeground': '#7d8590',
        'editorGutter.background': '#0d1117',
        'editorWidget.background': '#161b22',
        'editorWidget.border': '#30363d',
        'editorSuggestWidget.background': '#161b22',
        'editorSuggestWidget.border': '#30363d',
        'scrollbarSlider.background': '#484f5833',
        'scrollbarSlider.hoverBackground': '#484f5866',
        'editorBracketMatch.background': '#58a6ff33',
        'editorBracketMatch.border': '#58a6ff66',
        'editorIndentGuide.background': '#21262d',
        'editorIndentGuide.activeBackground': '#30363d',
      },
    })
  } catch {
    // Theme may already be defined by CodeEditor — that's fine.
  }
}

onMounted(() => {
  if (!container.value) return

  ensureTheme()

  const uri = monaco.Uri.parse(`file:///flint-sm-${props.language}-${Date.now()}.${props.language === 'json' ? 'json' : 'js'}`)
  const model = monaco.editor.createModel(props.modelValue, props.language, uri)

  editor.value = monaco.editor.create(container.value, {
    model,
    theme: 'flint-dark',
    readOnly: props.readonly,
    fontSize: 12,
    fontFamily: "'JetBrains Mono', 'Fira Mono', 'Consolas', monospace",
    fontLigatures: false,
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

  editor.value.onDidChangeModelContent(() => {
    if (internalUpdate) return
    internalUpdate = true
    emit('update:modelValue', model.getValue())
    internalUpdate = false
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

// Sync external model changes.
watch(() => props.modelValue, (val) => {
  if (internalUpdate) return
  const ed = editor.value
  if (!ed) return
  const model = ed.getModel()
  if (!model) return
  if (val !== model.getValue()) {
    internalUpdate = true
    model.setValue(val)
    internalUpdate = false
  }
})
</script>

<template>
  <div
    ref="container"
    class="w-full overflow-hidden border border-terminal-border rounded-sm flex-1 min-h-0"
    @keydown.stop
    @keyup.stop
    @keypress.stop
  />
</template>
