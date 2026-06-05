/**
 * SP3 Composer — Playwright visual QA
 *
 * Tests the Codex-grade composer action bar across light/dark modes:
 *   - rest state (bar visible with attach / model / effort / mode controls)
 *   - mode-menu open
 *   - model-menu open
 *   - slash-menu triggered by typing "/"
 *
 * Mirrors the harness in sp2-chat.spec.ts:
 *   - No webServer block — requires `npm run dev` on :5173
 *   - Theme seeded via localStorage addInitScript (key: dsc.theme)
 *   - /v1/models route-mocked so fixture works without a real gateway
 *   - Screenshots written to test-results/sp3/
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/sp3-composer.spec.ts
 */

import { Page, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const DIR = path.resolve(__dirname, '..', 'test-results', 'sp3')
if (!fs.existsSync(DIR)) fs.mkdirSync(DIR, { recursive: true })

async function setup(page: Page, mode: string) {
  await page.addInitScript((m) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })), mode)
  await page.route('**/v1/models', (r) =>
    r.fulfill({
      json: { models: ['deepseek-chat', 'deepseek-reasoner'], active: 'deepseek-chat', effort: 'high' },
    })
  )
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/?fixture=chat')
  await page.waitForSelector('.composer-wrap', { timeout: 15000 })
  await page.waitForTimeout(300)
}

for (const mode of ['light', 'dark'] as const) {
  test(`composer ${mode} rest`, async ({ page }) => {
    await setup(page, mode)
    await page.screenshot({ path: path.join(DIR, `composer-${mode}-rest.png`), fullPage: true })
  })

  test(`composer ${mode} mode-menu`, async ({ page }) => {
    await setup(page, mode)
    await page.getByTestId('mode-trigger').click()
    await page.waitForTimeout(120)
    await page.screenshot({ path: path.join(DIR, `composer-${mode}-mode.png`), fullPage: true })
  })

  test(`composer ${mode} model-menu`, async ({ page }) => {
    await setup(page, mode)
    await page.getByTestId('model-trigger').click()
    await page.waitForTimeout(120)
    await page.screenshot({ path: path.join(DIR, `composer-${mode}-model.png`), fullPage: true })
  })

  test(`composer ${mode} slash`, async ({ page }) => {
    await setup(page, mode)
    await page.getByTestId('composer-input').fill('/')
    await page.waitForTimeout(150)
    await page.screenshot({ path: path.join(DIR, `composer-${mode}-slash.png`), fullPage: true })
  })
}
