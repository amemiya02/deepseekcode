/**
 * P3.4 Capability-Driven Model + Effort Picker — Playwright behavioral QA
 *
 * Verifies that the composer's model and effort pickers are driven by the
 * /v1/models capability descriptor contract introduced in P3.4:
 *
 *   Scenario A (capability model):
 *     - Route /v1/models with a full descriptor (context:1000000, caps.reasoning:true,
 *       effort.kind:'levels' including 'max').
 *     - Open model picker → row shows '1M' context badge and reasoning glyph.
 *     - Open effort picker → 'max' is listed.
 *
 *   Scenario B (effort.kind:'none'):
 *     - Route /v1/models with effort.kind:'none' for the active model.
 *     - Effort trigger must be ABSENT (EffortSwitcher returns null when levels=[]).
 *
 * Harness mirrors p3.3b-mid-turn-steering.spec.ts:
 *   - No webServer block — requires `npm run dev` on :5173
 *   - Theme seeded via localStorage addInitScript (key: dsc.theme)
 *   - /v1/* endpoints route-mocked so the app mounts cleanly
 *   - Screenshots written to test-results/p3.4/
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/p3.4-model-capability.spec.ts
 */

import { expect, Page, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const DIR = path.resolve(__dirname, '..', 'test-results', 'p3.4')
if (!fs.existsSync(DIR)) fs.mkdirSync(DIR, { recursive: true })

/** Full capability descriptor response — effort.kind:'levels' including 'max'. */
const MODELS_WITH_CAPS = {
  active: 'deepseek-v4-flash',
  effort: 'max',
  models: [
    {
      id: 'deepseek-v4-flash',
      label: 'DeepSeek V4 Flash',
      provider: 'deepseek',
      caps: { vision: false, tools: true, reasoning: true },
      effort: { kind: 'levels', levels: ['low', 'medium', 'high', 'max'], default: 'max' },
      context: 1_000_000,
    },
    {
      id: 'deepseek-v4-pro',
      label: 'DeepSeek V4 Pro',
      provider: 'deepseek',
      caps: { vision: false, tools: true, reasoning: true },
      effort: { kind: 'levels', levels: ['low', 'medium', 'high', 'max'], default: 'max' },
      context: 1_000_000,
    },
  ],
}

/** Descriptor response where the active model has effort.kind:'none'. */
const MODELS_NO_EFFORT = {
  active: 'deepseek-v4-flash',
  effort: 'max',
  models: [
    {
      id: 'deepseek-v4-flash',
      label: 'DeepSeek V4 Flash',
      provider: 'deepseek',
      caps: { vision: false, tools: false, reasoning: false },
      effort: { kind: 'none' },
      context: 0,
    },
  ],
}

async function setupCommon(page: Page, mode: string, modelsPayload: object) {
  // Seed theme via localStorage before page load.
  await page.addInitScript(
    (m) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })),
    mode,
  )

  await page.route('**/v1/models', (r) => r.fulfill({ json: modelsPayload }))
  await page.route('**/v1/sessions', (r) => r.fulfill({ json: { sessions: [] } }))
  await page.route('**/v1/changed', (r) => r.fulfill({ json: { entries: [] } }))
  await page.route('**/v1/telemetry**', (r) => r.fulfill({ status: 200, body: '' }))
  await page.route('**/v1/diff**', (r) =>
    r.fulfill({ json: { path: 'x', patch: '' } }),
  )
  await page.route('**/v1/onboarding**', (r) =>
    r.fulfill({ json: { needsOnboarding: false } }),
  )
  await page.route('**/v1/update**', (r) =>
    r.fulfill({ json: { updateAvailable: false } }),
  )

  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/')
  await page.waitForSelector('[data-testid="app-shell"]', { timeout: 15_000 })
  await page.waitForTimeout(400)
}

// ---------------------------------------------------------------------------
// Scenario A — capability-rich descriptor
// ---------------------------------------------------------------------------
for (const mode of ['light', 'dark'] as const) {
  test(`capability picker ${mode} — model row shows 1M badge + reasoning glyph`, async ({
    page,
  }) => {
    await setupCommon(page, mode, MODELS_WITH_CAPS)

    // Open the model picker.
    const trigger = page.getByTestId('model-trigger')
    await expect(trigger).toBeVisible({ timeout: 5_000 })
    await trigger.click()

    // At least one row should have a context badge showing '1M'.
    const ctxBadge = page.getByTestId('model-ctx').first()
    await expect(ctxBadge).toBeVisible({ timeout: 3_000 })
    await expect(ctxBadge).toHaveText('1M')

    // The reasoning glyph must be present for the DeepSeek V4 Flash row.
    const reasoningGlyph = page.getByTestId('cap-reasoning').first()
    await expect(reasoningGlyph).toBeVisible({ timeout: 3_000 })

    await page.screenshot({ path: path.join(DIR, `model-picker-caps-${mode}.png`) })

    // Close picker before the next assertion.
    await page.keyboard.press('Escape')
    await page.waitForTimeout(150)
  })

  test(`capability picker ${mode} — effort picker lists max`, async ({ page }) => {
    await setupCommon(page, mode, MODELS_WITH_CAPS)

    // The effort trigger must be visible (effort.kind === 'levels' → non-empty levels).
    const effortTrigger = page.getByTestId('effort-trigger')
    await expect(effortTrigger).toBeVisible({ timeout: 5_000 })

    // Open the effort picker.
    await effortTrigger.click()

    // 'max' must appear as an option.
    const maxOption = page.locator('[role="listbox"] [role="option"]', { hasText: 'max' })
    await expect(maxOption).toBeVisible({ timeout: 3_000 })

    await page.screenshot({ path: path.join(DIR, `effort-picker-max-${mode}.png`) })
  })
}

// ---------------------------------------------------------------------------
// Scenario B — effort.kind:'none' → EffortSwitcher hidden
// ---------------------------------------------------------------------------
for (const mode of ['light', 'dark'] as const) {
  test(`capability picker ${mode} — effort trigger absent when kind is none`, async ({
    page,
  }) => {
    await setupCommon(page, mode, MODELS_NO_EFFORT)

    // EffortSwitcher returns null when levels=[] — the trigger must not exist.
    // Use $() so we can assert null without a timeout error.
    const el = await page.$('[data-testid="effort-trigger"]')
    expect(el).toBeNull()

    await page.screenshot({ path: path.join(DIR, `effort-hidden-${mode}.png`) })
  })
}
