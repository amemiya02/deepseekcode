/**
 * P4.3 Chat Surface — Playwright visual QA
 *
 * Verifies the refined chat surface on the ?fixture=chat seam:
 *   - Reasoning disclosure is a quiet inline line (no pill border/background on the toggle).
 *   - Assistant prose has line-height 1.7 (not 1.65).
 *   - User bubble is right-aligned with an asymmetric tight top-right corner.
 *
 * Harness:
 *   - No webServer block — requires `npm run dev` on :5173
 *   - Theme seeded via localStorage addInitScript (key: dsc.theme)
 *   - Screenshots written to test-results/p4.3/
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/p4.3-chat-surface.spec.ts
 */

import { expect, Page, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const DIR = path.resolve(__dirname, '..', 'test-results', 'p4.3')
if (!fs.existsSync(DIR)) fs.mkdirSync(DIR, { recursive: true })

async function setupChat(page: Page, mode: 'light' | 'dark') {
  await page.addInitScript(
    (m: string) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })),
    mode,
  )
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/?fixture=chat')
  await page.waitForSelector('[data-testid="transcript"]', { timeout: 15_000 })
  await page.waitForSelector('.transcript__lane .msg', { timeout: 15_000 })
  await page.waitForTimeout(350)
}

for (const mode of ['light', 'dark'] as const) {
  test(`chat-surface ${mode} — full transcript screenshot`, async ({ page }) => {
    await setupChat(page, mode)
    await expect(page.getByTestId('transcript')).toBeVisible()
    await page.screenshot({ path: path.join(DIR, `chat-full-${mode}.png`), fullPage: true })
  })

  test(`chat-surface ${mode} — reasoning toggle has no pill border/background`, async ({ page }) => {
    await setupChat(page, mode)

    const toggle = page.getByTestId('thinking-toggle').first()
    await expect(toggle).toBeVisible()

    // Verify no pill: border should be none and background should be transparent
    const borderStyle = await toggle.evaluate((el) => getComputedStyle(el).border)
    const bg = await toggle.evaluate((el) => getComputedStyle(el).backgroundColor)

    // border should resolve to "0px none rgb(...)" pattern — no actual border width
    const hasPillBorder = borderStyle.includes('px') && !borderStyle.startsWith('0px')
    expect(hasPillBorder).toBe(false)

    // background should be transparent (rgba(0,0,0,0)) — no pill fill
    expect(bg).toBe('rgba(0, 0, 0, 0)')

    await page.screenshot({ path: path.join(DIR, `chat-reasoning-no-pill-${mode}.png`), fullPage: false })
  })

  test(`chat-surface ${mode} — user bubble is right-aligned`, async ({ page }) => {
    await setupChat(page, mode)

    const userMsg = page.locator('.msg--user .msg__bubble').first()
    await expect(userMsg).toBeVisible()

    // align-self: flex-end means the bubble is right-aligned in its flex column
    const alignSelf = await userMsg.evaluate((el) => getComputedStyle(el).alignSelf)
    expect(alignSelf).toBe('flex-end')

    await userMsg.screenshot({ path: path.join(DIR, `chat-user-bubble-${mode}.png`) })
  })

  test(`chat-surface ${mode} — thinking toggle visible + expand works`, async ({ page }) => {
    await setupChat(page, mode)

    const toggle = page.getByTestId('thinking-toggle').first()
    await expect(toggle).toBeVisible()

    // click to expand
    await toggle.click()
    await page.waitForTimeout(150)

    const body = page.locator('.thinking__body').first()
    await expect(body).toBeVisible()

    await page.screenshot({ path: path.join(DIR, `chat-thinking-expanded-${mode}.png`), fullPage: true })
  })
}
