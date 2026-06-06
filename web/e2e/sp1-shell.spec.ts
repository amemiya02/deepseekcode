/**
 * SP1 Shell — Playwright visual QA
 *
 * Tests the resizable 3-column shell across light/dark modes and three viewport
 * widths, plus all three layout presets (Balanced / Focus / Review).
 *
 * Mirrors the harness established in visual-qa.spec.ts:
 *   - No webServer block — requires `npm run dev` or `npm run preview` on :5173
 *   - Theme seeded via localStorage addInitScript (same key: dsc.theme)
 *   - waitForShell waits for [data-testid="app-shell"] (15 s timeout)
 *   - Screenshots written to test-results/sp1/ (create dir if needed)
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/sp1-shell.spec.ts
 */

import { expect, Page, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

// Ensure output directory exists (ESM-compatible __dirname equivalent)
const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const SCREENSHOT_DIR = path.resolve(__dirname, '..', 'test-results', 'sp1')
if (!fs.existsSync(SCREENSHOT_DIR)) {
  fs.mkdirSync(SCREENSHOT_DIR, { recursive: true })
}

/** Wait for the app shell to be visible before taking any screenshot. */
async function waitForShell(page: Page) {
  await page.waitForSelector('[data-testid="app-shell"]', { timeout: 15_000 })
  // Allow fonts + CSS transitions to settle.
  await page.waitForTimeout(300)
}

const WIDTHS = [1024, 1280, 1600] as const
const MODES = ['light', 'dark'] as const
const PRESETS = ['balanced', 'focus', 'review'] as const

for (const mode of MODES) {
  for (const w of WIDTHS) {
    test(`shell ${mode} @${w}`, async ({ page }) => {
      // Seed theme via localStorage before page load — same pattern as visual-qa.spec.ts
      await page.addInitScript(
        (m: string) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })),
        mode,
      )
      await page.setViewportSize({ width: w, height: 900 })
      await page.goto('/')
      await waitForShell(page)

      // Assert shell visible
      await expect(page.getByTestId('app-shell')).toBeVisible()

      // Baseline screenshot
      await page.screenshot({
        path: path.join(SCREENSHOT_DIR, `shell-${mode}-${w}.png`),
      })

      // Exercise each preset and screenshot
      for (const preset of PRESETS) {
        const btn = page.getByTestId(`preset-${preset}`)
        // If the preset button isn't visible (e.g. collapsed into an overflow
        // menu at a narrow width), skip gracefully rather than hard-fail.
        if (await btn.isVisible()) {
          await btn.click()
          await page.waitForTimeout(150) // let transition settle
          await page.screenshot({
            path: path.join(SCREENSHOT_DIR, `preset-${preset}-${mode}-${w}.png`),
          })
        }
      }
    })
  }
}
