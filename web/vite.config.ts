import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: 'dist',
  },
  resolve: {
    conditions: ['browser'],
  },
  server: {
    proxy: {
      '/v1': {
        target: 'http://localhost:7432',
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
  },
})
