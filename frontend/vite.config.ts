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
    host: '0.0.0.0',
    port: 5173,
    proxy: {
      '/api': {
        target: process.env.VITE_API_TARGET || 'http://127.0.0.1:8080',
        changeOrigin: true,
        ws: true,
      },
    },
  },
})
