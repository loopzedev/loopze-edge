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
      // Dashboard WebSocket — must come BEFORE the generic /api rule
      // so vite picks the ws-capable proxy for the upgrade request.
      '/api/dashboard/ws': {
        target: 'ws://localhost:1880',
        ws: true,
      },
      '/api': {
        target: 'http://localhost:1880',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://localhost:1880',
        ws: true,
      },
      // Flow-defined HTTP endpoints (http-in nodes). Matches the
      // default LOOPZE_HTTP_NODE_ROOT. Operators running with a custom
      // prefix in production don't hit Vite — this is dev-only.
      '/endpoint': {
        target: 'http://localhost:1880',
        changeOrigin: true,
      },
      // Dashboard SPA mount path. Without this proxy, Vite would
      // serve frontend/dashboard/index.html directly (it exists on
      // disk) but resolve the SPA's /src/main.ts against the editor's
      // root — pulling editor JS into a dashboard HTML wrapper and
      // breaking everything. Proxying to the Go backend serves the
      // *built* dashboard bundle from web/dist-dashboard/. For
      // dashboard HMR, run `npm run dev:dashboard` on port 5174 and
      // open localhost:5174 directly.
      '/dashboard': {
        target: 'http://localhost:1880',
        changeOrigin: true,
      },
    },
  },
})
