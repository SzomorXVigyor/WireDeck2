import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  build: {
    target: 'es2022',
  },
  optimizeDeps: {
    esbuildOptions: {
      target: 'es2022',
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api/v1/iam': {
        target: 'http://127.0.0.1:8081',
        changeOrigin: true,
      },
      '/api/v1/webview': {
        target: 'http://127.0.0.1:8082',
        changeOrigin: true,
        ws: true,
      },
      '/api/v1/vnc': {
        target: 'http://127.0.0.1:8083',
        changeOrigin: true,
        ws: true,
      },
    },
  },
})
