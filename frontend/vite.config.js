import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Padrão: o gateway do docker compose, que roteia para todos os serviços.
const target = process.env.VITE_PROXY_TARGET || 'http://localhost'

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': target,
      '/ws': {
        target: target.replace(/^http/, 'ws'),
        ws: true,
      },
      '/musicas': target,
    },
  },
})
