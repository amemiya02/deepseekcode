/**
 * P3.3 Adaptive Companion Review Pane — Playwright visual + behavioral QA
 *
 * The right (review) pane is now an *adaptive companion* (spec §4) rather than
 * an always-present 3rd column. This spec drives the three adaptive states
 * across light + dark, route-mocking /v1/changed to control the working tree:
 *
 *   - revealed — /v1/changed non-empty → zone-workspace visible,
 *                app-shell[data-workspace-collapsed="false"] (3-column)
 *   - hidden   — /v1/changed empty → zone-workspace absent,
 *                app-shell[data-workspace-collapsed="true"] (2-column)
 *   - pin      — from the hidden state, clicking collapse-workspace pins the
 *                pane open despite no changes (reviewPin: 'open')
 *
 * The App effect fetches /v1/changed on mount (workspaceRefreshKey starts at 0,
 * so the effect fires on load) — the mock applies without needing a turn.
 *
 * Mirrors the harness in sp5-review.spec.ts:
 *   - No webServer block — requires `npm run dev` on :5173
 *   - Theme seeded via localStorage addInitScript (key: dsc.theme)
 *   - /v1/* endpoints route-mocked so the app mounts cleanly
 *   - Screenshots written to test-results/p3.3/
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/p3.3-adaptive-review.spec.ts
 */

import { expect, Page, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const DIR = path.resolve(__dirname, '..', 'test-results', 'p3.3')
if (!fs.existsSync(DIR)) fs.mkdirSync(DIR, { recursive: true })

// A single changed entry is enough to flip /v1/changed → non-empty (hasChanges).
const CHANGED_ENTRIES = [{ path: 'src/parser.ts', status: 'M', deleted: false }]

// Reused diff/file payloads so the ReviewPanel mounts cleanly when revealed.
const SAMPLE_PATCH = `--- a/src/parser.ts
+++ b/src/parser.ts
@@ -1,3 +1,4 @@
 export function parse(input: string) {
+  // adaptive review companion
   return input.trim()
 }`

const FILE_CONTENT = {
  path: 'src/parser.ts',
  content: 'export function parse(input: string) {\n  return input.trim()\n}\n',
  binary: false,
  truncated: false,
}

/**
 * Route-mock every /v1 endpoint the shell + review pane consume, with the
 * /v1/changed entries supplied by the caller so each test controls hasChanges.
 */
async function setup(page: Page, mode: string, changed: Array<Record<string, unknown>>) {
  // Seed theme via localStorage before page load.
  await page.addInitScript((m) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })), mode)

  await page.route('**/v1/models', (r) =>
    r.fulfill({
      json: { models: ['deepseek-chat', 'deepseek-reasoner'], active: 'deepseek-chat', effort: 'high' },
    })
  )
  await page.route('**/v1/sessions', (r) => r.fulfill({ json: { sessions: [] } }))
  // The variable bit: changed entries drive the adaptive reveal.
  await page.route('**/v1/changed', (r) => r.fulfill({ json: { entries: changed } }))
  await page.route('**/v1/diff**', (r) => {
    const url = new URL(r.request().url())
    const filePath = url.searchParams.get('path') ?? 'unknown'
    r.fulfill({ json: { path: filePath, patch: SAMPLE_PATCH } })
  })
  await page.route('**/v1/file**', (r) => r.fulfill({ json: FILE_CONTENT }))
  await page.route('**/v1/telemetry**', (r) => r.fulfill({ status: 200, body: '' }))

  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/')

  // The shell root always renders (2- or 3-column) — wait on it, NOT on
  // review-hero (which is absent in the hidden 2-column state).
  await page.waitForSelector('[data-testid="app-shell"]', { timeout: 15_000 })
  await page.waitForTimeout(400)
}

for (const mode of ['light', 'dark'] as const) {
  test(`adaptive ${mode} — review pane reveals when the working tree has changes`, async ({ page }) => {
    await setup(page, mode, CHANGED_ENTRIES)

    // Non-empty /v1/changed → auto-reveal: workspace zone present, 3-column.
    await expect(page.getByTestId('zone-workspace')).toBeVisible()
    expect(await page.getByTestId('app-shell').getAttribute('data-workspace-collapsed')).toBe('false')

    await page.getByTestId('app-shell').screenshot({ path: path.join(DIR, `review-revealed-${mode}.png`) })
  })

  test(`adaptive ${mode} — review pane hidden with a clean working tree (2 columns)`, async ({ page }) => {
    await setup(page, mode, [])

    // Empty /v1/changed → auto-hide: workspace zone absent, 2-column.
    expect(await page.$('[data-testid="zone-workspace"]')).toBeNull()
    expect(await page.getByTestId('app-shell').getAttribute('data-workspace-collapsed')).toBe('true')

    await page.getByTestId('app-shell').screenshot({ path: path.join(DIR, `review-hidden-${mode}.png`) })
  })

  test(`adaptive ${mode} — workspace toggle pins the pane open despite no changes`, async ({ page }) => {
    await setup(page, mode, [])

    // Starts hidden (clean tree).
    expect(await page.$('[data-testid="zone-workspace"]')).toBeNull()
    expect(await page.getByTestId('app-shell').getAttribute('data-workspace-collapsed')).toBe('true')

    // Clicking the collapse-workspace tab pins reviewPin: 'open'.
    await page.getByTestId('collapse-workspace').click()
    await page.waitForTimeout(300)

    // Pinned open even though /v1/changed is empty → workspace zone appears, 3-column.
    await expect(page.getByTestId('zone-workspace')).toBeVisible()
    expect(await page.getByTestId('app-shell').getAttribute('data-workspace-collapsed')).toBe('false')

    await page.getByTestId('app-shell').screenshot({ path: path.join(DIR, `review-pinned-${mode}.png`) })
  })
}
