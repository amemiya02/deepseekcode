/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
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
    environmentOptions: {
      jsdom: {
        url: 'http://localhost/',
      },
    },
    globals: true,
    setupFiles: ['./vitest-setup.ts'],
    // Exclude Playwright e2e specs — those run via `npx playwright test`, not vitest.
    exclude: ['**/node_modules/**', '**/e2e/**'],
  },
})
