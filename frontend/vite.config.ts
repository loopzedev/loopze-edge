import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  // Relative base so emitted asset URLs resolve against the runtime
  // <base href> the Go backend injects into index.html. This is what
  // enables a single build to be served at "/" or under any subpath.
  base: './',
  plugins: [vue()],
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
