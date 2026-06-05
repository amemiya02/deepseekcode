/**
 * P3.1 Island Material QA — Playwright visual QA
 *
 * Asserts the Direction-C "material rule": code/diff islands are ALWAYS
 * obsidian (#0b0d13 = rgb(11, 13, 19)) in BOTH light AND dark app modes.
 *
 * Two fixtures are exercised:
 *   1. ?fixture=chat — transcript with a fenced code block → `.island` from CodeBlock/CodeIsland
 *   2. Review panel  — diff via /v1/diff route-mock → `.island` from DiffView/DiffIsland
 *
 * Mirrors the harness established in sp5-review.spec.ts:
 *   - No webServer block — requires `npm run dev` on :5173
 *   - Theme seeded via localStorage addInitScript (key: dsc.theme)
 *   - /v1/models + /v1/sessions + /v1/changed + /v1/diff + /v1/file + /v1/telemetry route-mocked
 *   - Screenshots written to test-results/p3.1/
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/p3.1-island-material.spec.ts
 */

import { expect, Page, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const DIR = path.resolve(__dirname, '..', 'test-results', 'p3.1')
if (!fs.existsSync(DIR)) fs.mkdirSync(DIR, { recursive: true })

// The obsidian island background — rgb(11, 13, 19) = #0b0d13.
const OBSIDIAN = 'rgb(11, 13, 19)'

// A minimal two-hunk patch so diff-added / diff-removed testids appear.
const SAMPLE_PATCH = `@@ -1,2 +1,2 @@
-old line
+new line
 keep
@@ -5,1 +5,1 @@
-foo
+bar`

const CHANGED_ENTRIES = [
  { path: 'internal/gateway/workspace.go', status: 'M', deleted: false },
]

async function setupReview(page: Page, mode: string) {
  await page.addInitScript((m) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })), mode)

  await page.route('**/v1/models', (r) =>
    r.fulfill({ json: { models: ['deepseek-chat'], active: 'deepseek-chat', effort: 'high' } })
  )
  await page.route('**/v1/sessions', (r) => r.fulfill({ json: { sessions: [] } }))
  await page.route('**/v1/changed', (r) => r.fulfill({ json: { entries: CHANGED_ENTRIES } }))
  await page.route('**/v1/diff**', (r) => {
    const url = new URL(r.request().url())
    const filePath = url.searchParams.get('path') ?? 'unknown'
    r.fulfill({ json: { path: filePath, patch: SAMPLE_PATCH } })
  })
  await page.route('**/v1/file**', (r) =>
    r.fulfill({ json: { path: 'file.go', content: '// hello', binary: false, truncated: false } })
  )
  await page.route('**/v1/telemetry**', (r) => r.fulfill({ status: 200, body: '' }))

  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/')
  await page.waitForSelector('[data-testid="review-hero"]', { timeout: 15_000 })
  await page.waitForTimeout(400)
}

for (const mode of ['light', 'dark'] as const) {
  // ── Chat fixture: code island is obsidian in both modes ──────────────────
  test(`island ${mode} — chat fixture: code island is obsidian`, async ({ page }) => {
    await page.addInitScript((m: string) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })), mode)
    await page.setViewportSize({ width: 1280, height: 900 })
    await page.goto('/?fixture=chat')
    await page.waitForSelector('[data-testid="transcript"]', { timeout: 15_000 })
    await page.waitForSelector('[data-island]', { timeout: 10_000 })
    await page.waitForTimeout(350)

    // Island background must be obsidian in BOTH modes (material rule).
    const island = page.locator('[data-island]').first()
    await expect(island).toBeVisible()
    const bg = await island.evaluate((el) => getComputedStyle(el).backgroundColor)
    expect(bg, `[${mode}] island background`).toBe(OBSIDIAN)

    // Island bar must exist and its color must NOT be the light body text color.
    const bar = island.locator('.island__bar').first()
    await expect(bar).toBeVisible()
    const barColor = await bar.evaluate((el) => getComputedStyle(el).color)
    // Light body text in paper mode would be near-black; island accent is the brand blue.
    // We just assert it is NOT a near-black color (rgb(13,16,22) is the paper text token).
    expect(barColor, `[${mode}] island bar color should not be paper text`).not.toBe('rgb(13, 16, 22)')

    await page.screenshot({ path: path.join(DIR, `island-${mode}.png`), fullPage: false })
  })

  // ── Review fixture: diff island is obsidian + diff-added/removed present ─
  test(`island ${mode} — diff island is obsidian, +N −M present`, async ({ page }) => {
    await setupReview(page, mode)

    // Click the first changed-file row to load the diff.
    const firstRow = page.getByTestId('change-status').first()
    await firstRow.click()
    await page.waitForTimeout(500)

    // Diff hunk must be visible.
    await expect(page.locator('[data-testid="diff-hunk"]').first()).toBeVisible({ timeout: 5_000 })

    // diff island must be obsidian.
    const diffIsland = page.locator('[data-island]').first()
    await expect(diffIsland).toBeVisible()
    const diffBg = await diffIsland.evaluate((el) => getComputedStyle(el).backgroundColor)
    expect(diffBg, `[${mode}] diff island background`).toBe(OBSIDIAN)

    // +N / −M counts must be present.
    await expect(page.getByTestId('diff-added')).toBeVisible()
    await expect(page.getByTestId('diff-removed')).toBeVisible()

    await page.screenshot({ path: path.join(DIR, `diff-island-${mode}.png`), fullPage: false })
  })
}
