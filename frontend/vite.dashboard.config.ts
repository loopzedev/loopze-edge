import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

// Dashboard SPA build. Separate from vite.config.ts (the editor build)
// because:
//   - Different audience: operators / kiosks vs. flow authors.
//   - Different bundle budget: no Vue Flow, no Monaco, no editor stores.
//   - Different output dir: web/dist-dashboard/, embedded via a second
//     embed.FS in web/embed.go.
//
// Both configs share the same plugins and the same @/ alias (which here
// resolves to dashboard/src so dashboard components can import each
// other with @/-prefixed paths).
export default defineConfig({
  root: resolve(__dirname, 'dashboard'),
  // Absolute base so /dashboard/index.html (no trailing slash) still
  // resolves assets correctly. With "./" the browser would try
  // /assets/... when the URL ends without a slash, hitting the
  // editor SPA's catch-all fallback (and its login modal).
  base: '/dashboard/',
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'dashboard/src'),
    },
  },
  build: {
    // Path relative to `root` above. The two `..` hop out of
    // frontend/dashboard back to frontend/, then into web/dist-dashboard.
    outDir: resolve(__dirname, '../web/dist-dashboard'),
    emptyOutDir: true,
  },
  server: {
    port: 5174,
    proxy: {
      '/api': {
        target: 'http://localhost:1880',
        changeOrigin: true,
      },
      '/api/dashboard/ws': {
        target: 'ws://localhost:1880',
        ws: true,
      },
    },
  },
})
