import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 前端产物会被 Go 通过 go:embed 内嵌；开发时用 8787 端口的后端做代理。
export default defineConfig({
  plugins: [react()],
  base: '/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1200,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8787',
    },
  },
})
