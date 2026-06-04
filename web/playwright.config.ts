import { defineConfig, devices } from '@playwright/test'

/**
 * Playwright configuration for Phase 5 visual QA.
 * Screenshots are taken per surface in light + dark, gating regressions.
 * Run: npx playwright test (requires `npm run dev` or preview server running on port 5173)
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? 'http://localhost:5173',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium-light',
      use: {
        ...devices['Desktop Chrome'],
        colorScheme: 'light',
        viewport: { width: 1440, height: 900 },
      },
    },
    {
      name: 'chromium-dark',
      use: {
        ...devices['Desktop Chrome'],
        colorScheme: 'dark',
        viewport: { width: 1440, height: 900 },
      },
    },
  ],
  // No webServer block — CI must start `npm run preview` (after build) externally,
  // or developers run `npm run dev` before this suite.
})
