/**
 * P3.2 Approval Gate QA — Playwright visual QA
 *
 * Asserts the spec §5 routing: an EDIT permission request (edit_file / write_file /
 * apply_patch) renders as an INLINE obsidian diff approval gate in the conversation,
 * while a non-edit request (bash command) renders the INLINE command card — there is
 * no blocking PermissionModal anywhere anymore (Codex/Claude-style inline approvals).
 *
 * Two fixtures are exercised in BOTH light AND dark app modes:
 *   1. ?fixture=approval      — edit_file → [data-testid="approval-gate"] containing a
 *      `.island` whose computed background is obsidian rgb(11, 13, 19); shows the path
 *      `src/parser.ts`; approve-once / approve-deny buttons present; NO modal.
 *   2. ?fixture=approval-cmd  — bash → the same approval-gate, now containing the
 *      [data-testid="approval-cmd"] command card with the bare command; NO modal.
 *
 * Mirrors the harness established in sp5-review.spec.ts / p3.1-island-material.spec.ts:
 *   - No webServer block — requires `npm run dev` on :5173
 *   - Theme seeded via localStorage addInitScript (key: dsc.theme)
 *   - /v1/models + /v1/sessions + /v1/changed + /v1/diff + /v1/file + /v1/telemetry route-mocked
 *   - chromium-light + chromium-dark projects (playwright.config.ts)
 *   - Screenshots written to test-results/p3.2/
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/p3.2-approval-gate.spec.ts
 */

import { expect, Page, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const DIR = path.resolve(__dirname, '..', 'test-results', 'p3.2')
if (!fs.existsSync(DIR)) fs.mkdirSync(DIR, { recursive: true })

// The obsidian island background — rgb(11, 13, 19) = #0b0d13 (P3.1 material rule).
const OBSIDIAN = 'rgb(11, 13, 19)'

// Route-mock the gateway endpoints the app touches at boot, so the shell mounts
// without a live backend. The permission itself is seeded via the ?fixture= DEV seam.
async function setup(page: Page, mode: string) {
  await page.addInitScript((m) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })), mode)

  await page.route('**/v1/models', (r) =>
    r.fulfill({ json: { models: ['deepseek-chat'], active: 'deepseek-chat', effort: 'high' } })
  )
  await page.route('**/v1/sessions', (r) => r.fulfill({ json: { sessions: [] } }))
  await page.route('**/v1/changed', (r) => r.fulfill({ json: { entries: [] } }))
  await page.route('**/v1/diff**', (r) => {
    const url = new URL(r.request().url())
    const filePath = url.searchParams.get('path') ?? 'unknown'
    r.fulfill({ json: { path: filePath, patch: '' } })
  })
  await page.route('**/v1/file**', (r) =>
    r.fulfill({ json: { path: 'file.ts', content: '// hello', binary: false, truncated: false } })
  )
  await page.route('**/v1/telemetry**', (r) => r.fulfill({ status: 200, body: '' }))

  await page.setViewportSize({ width: 1280, height: 900 })
}

for (const mode of ['light', 'dark'] as const) {
  // ── Edit approval: inline obsidian diff gate in both modes ────────────────
  test(`approval ${mode} — edit_file renders the inline obsidian diff gate`, async ({ page }) => {
    await setup(page, mode)
    await page.goto('/?fixture=approval')

    // The inline gate must mount (the ?fixture=approval seam seeds an edit_file request).
    const gate = page.getByTestId('approval-gate')
    await expect(gate).toBeVisible({ timeout: 15_000 })

    // The gate contains the obsidian diff island — dark in BOTH light and dark modes.
    const island = gate.locator('.island').first()
    await expect(island).toBeVisible()
    const bg = await island.evaluate((el) => getComputedStyle(el).backgroundColor)
    expect(bg, `[${mode}] approval-gate island background`).toBe(OBSIDIAN)

    // The proposed change targets src/parser.ts (the fixture path) — shown in the
    // island header label (.diffview__path); the synthetic hunk header echoes it too.
    await expect(gate.locator('.diffview__path')).toHaveText('src/parser.ts')

    // The four-level action bar exposes accept-once + reject directly.
    await expect(gate.getByTestId('approve-once')).toBeVisible()
    await expect(gate.getByTestId('approve-deny')).toBeVisible()

    // The generic PermissionModal must NOT be shown for an edit approval.
    expect(await page.$('[data-testid="perm-backdrop"]')).toBeNull()
    expect(await page.$('[role="dialog"][aria-label="permission request"]')).toBeNull()

    // Let the mount/transition settle so the PNG captures the rendered gate, not a frame.
    await page.waitForTimeout(400)
    await page.screenshot({ path: path.join(DIR, `approval-gate-${mode}.png`), fullPage: false })
  })

  // ── Command approval: the same inline gate, with the command card ─────────
  test(`approval ${mode} — bash command renders the inline command card`, async ({ page }) => {
    await setup(page, mode)
    await page.goto('/?fixture=approval-cmd')

    // The inline gate mounts with the command preview card (never a modal).
    const gate = page.getByTestId('approval-gate')
    await expect(gate).toBeVisible({ timeout: 15_000 })
    await expect(gate.getByTestId('approval-cmd')).toBeVisible()
    await expect(gate.locator('.approval__tool')).toHaveText('bash')
    await expect(gate.locator('.approval__cmd')).toHaveText('rm -rf build/')

    // The action bar exposes allow-once + reject directly.
    await expect(gate.getByTestId('approve-once')).toBeVisible()
    await expect(gate.getByTestId('approve-deny')).toBeVisible()

    // The screen-blanking PermissionModal is gone for every tool.
    expect(await page.$('[data-testid="perm-backdrop"]')).toBeNull()
    expect(await page.$('[role="dialog"][aria-label="permission request"]')).toBeNull()

    // Let the mount/transition settle so the PNG captures the rendered card.
    await page.waitForTimeout(400)
    await page.screenshot({ path: path.join(DIR, `approval-cmd-${mode}.png`), fullPage: false })
  })
}
