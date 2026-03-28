import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  server: {
    port: 5173,
    proxy: {
      '/api/manage': 'http://localhost:8080',
    },
  },
  build: {
    outDir: '../internal/web/static/manage',
    emptyOutDir: true,
  },
})
