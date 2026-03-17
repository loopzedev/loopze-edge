import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import monacoEditorPlugin from 'vite-plugin-monaco-editor'
import { resolve } from 'path'

export default defineConfig({
  plugins: [
    vue(),
    // Only include JavaScript/TypeScript language support — skip all other languages.
    (monacoEditorPlugin as any).default({
      languageWorkers: ['editorWorkerService', 'typescript'],
    }),
  ],
  resolve: {
    alias: {
      '@': resolve(__dirname, './src'),
    },
  },
  build: {
    outDir: '../web/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:1880',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://localhost:1880',
        ws: true,
      },
    },
  },
})
