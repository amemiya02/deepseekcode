/**
 * SP2 Chat Surface — Playwright visual QA
 *
 * Tests the chat transcript surface across light/dark modes and three viewport
 * widths, plus a thinking-expanded + copy-state test.
 *
 * Mirrors the harness established in sp1-shell.spec.ts:
 *   - No webServer block — requires `npm run dev` on :5173
 *   - Theme seeded via localStorage addInitScript (key: dsc.theme)
 *   - waitForChat waits for [data-testid="transcript"] + .transcript__lane .msg
 *   - Screenshots written to test-results/sp2/ (create dir if needed)
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/sp2-chat.spec.ts
 */

import { expect, Page, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

// Ensure output directory exists (ESM-compatible __dirname equivalent)
const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const DIR = path.resolve(__dirname, '..', 'test-results', 'sp2')
if (!fs.existsSync(DIR)) fs.mkdirSync(DIR, { recursive: true })

async function waitForChat(page: Page) {
  await page.waitForSelector('[data-testid="transcript"]', { timeout: 15_000 })
  await page.waitForSelector('.transcript__lane .msg', { timeout: 15_000 })
  await page.waitForTimeout(350)
}

const WIDTHS = [1024, 1280, 1600] as const
const MODES = ['light', 'dark'] as const

for (const mode of MODES) {
  for (const w of WIDTHS) {
    test(`chat ${mode} @${w}`, async ({ page }) => {
      await page.addInitScript((m: string) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })), mode)
      await page.setViewportSize({ width: w, height: 900 })
      await page.goto('/?fixture=chat')
      await waitForChat(page)
      await expect(page.getByTestId('transcript')).toBeVisible()
      await page.screenshot({ path: path.join(DIR, `chat-${mode}-${w}.png`), fullPage: true })
    })
  }
}

test('thinking expanded + copy state (light @1280)', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('dsc.theme', JSON.stringify({ mode: 'light' })))
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/?fixture=chat')
  await waitForChat(page)
  await page.getByTestId('thinking-toggle').first().click()
  await page.waitForTimeout(150)
  await page.screenshot({ path: path.join(DIR, 'chat-thinking-expanded.png'), fullPage: true })
  const copy = page.getByTestId('codecard-copy').first()
  await copy.scrollIntoViewIfNeeded()
  await copy.click()
  await page.waitForTimeout(150)
  await page.screenshot({ path: path.join(DIR, 'chat-copied.png'), fullPage: true })
})
