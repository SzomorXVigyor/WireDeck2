import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

function generateBuildVersion(now: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  const yy = String(now.getUTCFullYear()).slice(-2)
  const mm = pad(now.getUTCMonth() + 1)
  const dd = pad(now.getUTCDate())
  const hh = pad(now.getUTCHours())
  const min = pad(now.getUTCMinutes())
  return `v${yy}.${mm}-${dd}${hh}${min}`
}

const appVersion =
  (process.env.APP_VERSION || process.env.VITE_APP_VERSION || '').trim() || generateBuildVersion(new Date())

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
  },
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
