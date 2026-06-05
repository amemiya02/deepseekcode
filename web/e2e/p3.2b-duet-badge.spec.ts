/**
 * P3.2b Duet Badge QA — Playwright visual QA
 *
 * Asserts the Duet pro-validation result surfaces as a quiet inline badge in the
 * transcript (spec §5.1). The `?fixture=duet` transcript seeds two duet items:
 *   1. decision "allow" → ✓ Validated by Pro (NO `duet--warn`), shows its reason
 *   2. decision "deny"  → ⚠ Pro proposed changes (HAS `duet--warn`)
 * Both states must render in BOTH light AND dark app modes.
 *
 * Mirrors the harness established in p3.1-island-material.spec.ts:
 *   - No webServer block — requires `npm run dev` on :5173
 *   - Theme seeded via localStorage addInitScript (key: dsc.theme)
 *   - chromium-light / chromium-dark projects (playwright.config.ts)
 *   - Screenshots written to test-results/p3.2b/
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/p3.2b-duet-badge.spec.ts
 */

import { expect, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const DIR = path.resolve(__dirname, '..', 'test-results', 'p3.2b')
if (!fs.existsSync(DIR)) fs.mkdirSync(DIR, { recursive: true })

for (const mode of ['light', 'dark'] as const) {
  test(`duet ${mode} — badge ok (allow) + warn (deny) render in the transcript`, async ({ page }) => {
    await page.addInitScript((m: string) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })), mode)
    await page.setViewportSize({ width: 1280, height: 900 })
    await page.goto('/?fixture=duet')
    await page.waitForSelector('[data-testid="transcript"]', { timeout: 15_000 })
    await page.waitForSelector('[data-testid="duet-badge"]', { timeout: 10_000 })
    await page.waitForTimeout(350)

    // Both duet badges from the fixture must be present.
    const badges = page.locator('[data-testid="duet-badge"]')
    await expect(badges, `[${mode}] two duet badges`).toHaveCount(2)

    // First badge = decision "allow" → ✓ Validated by Pro: NOT a warning, shows its reason.
    const ok = badges.nth(0)
    await expect(ok).toBeVisible()
    await expect(ok, `[${mode}] allow badge is not a warning`).not.toHaveClass(/duet--warn/)
    await expect(ok).toContainText('rm targets a regenerable build dir; safe.')

    // Second badge = decision "deny" → ⚠ Pro proposed changes: a warning.
    const warn = badges.nth(1)
    await expect(warn).toBeVisible()
    await expect(warn, `[${mode}] deny badge is a warning`).toHaveClass(/duet--warn/)

    await page.screenshot({ path: path.join(DIR, `duet-${mode}.png`), fullPage: false })
  })
}
