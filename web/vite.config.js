import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Builds into web/dist which the Go binary embeds. Dev server proxies /api to
// the running Go backend so `npm run dev` works against a local server.
export default defineConfig({
  plugins: [vue()],
  base: '/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8787',
        changeOrigin: true,
      },
    },
  },
})
