/**
 * P3.5 Appearance live-theming + status pressure — Playwright behavioral QA
 *
 * Verifies that the P3.5 theming engine surfaces correctly in Settings →
 * Appearance and that the StatusBar ctx-pressure color class responds to
 * ctxPct values. All assertions are behavioral (CSS custom-property values +
 * DOM class/attribute checks); no pixel-snapshot diffing so the suite is
 * deterministic across environments.
 *
 * Scenarios covered:
 *   A — Brand default: --accent is #4d6bfe initially (indigo = brand default).
 *   B — Live accent: switching to emerald changes --accent away from brand blue.
 *   C — Live UI font: switching to System updates --type-sans to contain system-ui.
 *   D — Live mode: switching mode via the Mode picker updates data-mode on <html>.
 *   E — ctx-pressure: StatusBar carries no pressure class at low ctx, ctxWarn at
 *       mid, ctxDanger at high (covered via unit tests; assertion here guards the
 *       rendered DOM class names via a fixture if available, else documents that
 *       ctx-pressure is unit-only for e2e).
 *
 * Harness mirrors p3.4-model-capability.spec.ts:
 *   - No webServer block — requires `npm run dev` on :5173
 *   - Theme seeded via localStorage addInitScript (key: dsc.theme)
 *   - /v1/* endpoints route-mocked so the app mounts cleanly
 *   - Screenshots written to test-results/p3.5/
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/p3.5-appearance-theming.spec.ts
 */

import { expect, Page, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const DIR = path.resolve(__dirname, '..', 'test-results', 'p3.5')
if (!fs.existsSync(DIR)) fs.mkdirSync(DIR, { recursive: true })

/** Minimal /v1/config payload — accent:indigo, density:comfortable. */
const CONFIG_DEFAULT = {
  accent: 'indigo',
  density: 'comfortable',
  model: 'deepseek-v4-flash',
  apiKey: '',
  baseURL: '',
}

/** Minimal /v1/models payload so the composer mounts cleanly. */
const MODELS_PAYLOAD = {
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
  ],
}

/**
 * Route-mock every /v1 endpoint the shell consumes and seed the theme via
 * localStorage so the app mounts cleanly in the given mode.
 */
async function setup(page: Page, mode: 'light' | 'dark') {
  // Seed theme before load so ThemeProvider reads it on first render.
  await page.addInitScript(
    (m) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m, accent: 'indigo' })),
    mode,
  )

  // Handle both GET (fetchConfig) and PUT (saveConfig) for /v1/config.
  await page.route('**/v1/config', (r) => {
    // Echo back a merged config so saveConfig succeeds and AppearanceSection
    // doesn't enter error state (which would un-mount the pickers).
    if (r.request().method() === 'PUT') {
      const body = r.request().postDataJSON() as Partial<typeof CONFIG_DEFAULT>
      r.fulfill({ json: { ...CONFIG_DEFAULT, ...body } })
    } else {
      r.fulfill({ json: CONFIG_DEFAULT })
    }
  })
  await page.route('**/v1/models', (r) => r.fulfill({ json: MODELS_PAYLOAD }))
  await page.route('**/v1/sessions', (r) => r.fulfill({ json: { sessions: [] } }))
  await page.route('**/v1/changed', (r) => r.fulfill({ json: { entries: [] } }))
  await page.route('**/v1/telemetry**', (r) => r.fulfill({ status: 200, body: '' }))
  await page.route('**/v1/diff**', (r) => r.fulfill({ json: { path: 'x', patch: '' } }))
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

/**
 * Open Settings → Appearance so the section pickers are visible.
 */
async function openAppearance(page: Page) {
  // Open the Settings window via the TitleBar gear button.
  await page.getByTestId('open-settings').click()
  await page.waitForTimeout(200)

  // Click the "Appearance" nav button in the settings rail.
  await page.getByRole('button', { name: /appearance/i }).click()
  await page.waitForTimeout(200)

  // Wait until at least the accent picker is mounted (AppearanceSection loaded
  // its config via fetchConfig).
  await page.waitForSelector('[data-testid="appearance-accent"]', { timeout: 8_000 })
}

// ---------------------------------------------------------------------------
// Suite A+B+C+D — live accent / font / mode, light + dark
// ---------------------------------------------------------------------------
for (const mode of ['light', 'dark'] as const) {
  test(`appearance ${mode} — brand default accent is #4d6bfe`, async ({ page }) => {
    await setup(page, mode)

    // Read --accent on <html> before opening settings (ThemeProvider applies it
    // synchronously on first render from the seeded localStorage).
    const accent = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--accent').trim(),
    )
    expect(accent).toBe('#4d6bfe')

    await page.screenshot({ path: path.join(DIR, `brand-accent-${mode}.png`) })
  })

  test(`appearance ${mode} — switching accent to emerald changes --accent away from brand blue`, async ({
    page,
  }) => {
    await setup(page, mode)
    await openAppearance(page)

    // Capture the brand-default accent for comparison.
    const before = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--accent').trim(),
    )
    expect(before).toBe('#4d6bfe')

    // Open the accent BrandedSelect and click "emerald".
    const accentTrigger = page.getByTestId('appearance-accent')
    await accentTrigger.click()

    // The listbox should now be open — pick emerald (role=option).
    const emeraldOption = page.locator('[role="listbox"] [role="option"]', {
      hasText: /emerald/i,
    })
    await expect(emeraldOption).toBeVisible({ timeout: 3_000 })
    // The backdrop div (position:fixed, z-index:10) intercepts Playwright's
    // coordinate-based click even with force:true. Dispatch via JS so React's
    // synthetic event fires directly on the option element.
    await page.evaluate(() => {
      const opt = Array.from(document.querySelectorAll('[role="option"]')).find(
        (o) => o.textContent?.toLowerCase().includes('emerald'),
      )
      ;(opt as HTMLElement | undefined)?.click()
    })
    // Wait for the ThemeProvider to apply the new --accent token (saveConfig
    // round-trip + React re-render).
    await page.waitForTimeout(600)

    // --accent must have changed from the brand blue.
    const after = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--accent').trim(),
    )
    expect(after).not.toBe('#4d6bfe')
    // emerald has hue 158 — the OKLCH value must reference that hue.
    expect(after).toContain('158')

    await page.screenshot({ path: path.join(DIR, `emerald-accent-${mode}.png`) })
  })

  test(`appearance ${mode} — switching UI font to System updates --type-sans`, async ({
    page,
  }) => {
    await setup(page, mode)
    await openAppearance(page)

    // Capture the brand-default UI font (IBM Plex Sans family).
    const before = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--type-sans').trim(),
    )
    expect(before).toContain('IBM Plex Sans')

    // Open the UI font BrandedSelect and pick "System".
    const uiFontTrigger = page.getByTestId('appearance-uifont')
    await uiFontTrigger.click()

    const systemOption = page.locator('[role="listbox"] [role="option"]', {
      hasText: /system/i,
    })
    await expect(systemOption).toBeVisible({ timeout: 3_000 })
    // Dispatch via JS to bypass the backdrop interceptor (same pattern as accent picker).
    await page.evaluate(() => {
      const opt = Array.from(document.querySelectorAll('[role="option"]')).find(
        (o) => o.textContent?.toLowerCase().includes('system'),
      )
      ;(opt as HTMLElement | undefined)?.click()
    })
    await page.waitForTimeout(400)

    // --type-sans must now contain system-ui.
    const after = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--type-sans').trim(),
    )
    expect(after).toContain('system-ui')

    await page.screenshot({ path: path.join(DIR, `system-font-${mode}.png`) })
  })

  test(`appearance ${mode} — switching mode via Mode picker updates data-mode`, async ({
    page,
  }) => {
    await setup(page, mode)
    await openAppearance(page)

    // Read current data-mode from <html>.
    const before = await page.evaluate(() =>
      document.documentElement.getAttribute('data-mode'),
    )
    expect(before).toBe(mode)

    // Open the Mode picker and pick the opposite mode.
    const targetMode = mode === 'light' ? 'dark' : 'light'
    const modeTrigger = page.getByTestId('appearance-mode')
    await modeTrigger.click()

    const targetOption = page.locator('[role="listbox"] [role="option"]', {
      hasText: new RegExp(targetMode, 'i'),
    })
    await expect(targetOption).toBeVisible({ timeout: 3_000 })
    // Dispatch via JS to bypass the backdrop interceptor (same pattern as accent picker).
    await page.evaluate((tgt) => {
      const opt = Array.from(document.querySelectorAll('[role="option"]')).find(
        (o) => o.textContent?.trim().toLowerCase() === tgt,
      )
      ;(opt as HTMLElement | undefined)?.click()
    }, targetMode)
    await page.waitForTimeout(400)

    // data-mode on <html> must reflect the new mode.
    const after = await page.evaluate(() =>
      document.documentElement.getAttribute('data-mode'),
    )
    expect(after).toBe(targetMode)

    await page.screenshot({ path: path.join(DIR, `mode-switch-${mode}.png`) })
  })
}

// ---------------------------------------------------------------------------
// Scenario E — ctx-pressure color (unit tests are the primary gate; this e2e
// assertion guards the StatusBar rendered output by navigating away from
// Settings to the main shell where StatusBar is always visible).
// ---------------------------------------------------------------------------
//
// The StatusBar ctx% is fed from the SSE telemetry stream in production. In
// e2e we cannot drive a real SSE payload reliably, so ctx-pressure color is
// covered definitively in StatusBar.test.tsx (unit) which checks the class
// applied to [data-testid="ctx-pct"]. This note is the documented rationale
// so the absence of a live e2e pressure assertion is intentional, not an
// oversight.
//
// We do include a smoke-test that the status bar is present and the ctx-pct
// segment renders without errors at the default idle state (ctxPct=0 → no
// pressure class).
for (const mode of ['light', 'dark'] as const) {
  test(`status bar ${mode} — ctx-pct segment renders with no pressure class at idle`, async ({
    page,
  }) => {
    await setup(page, mode)

    // The StatusBar is always visible in the shell footer.
    const ctxSeg = page.getByTestId('ctx-pct')
    await expect(ctxSeg).toBeVisible({ timeout: 5_000 })

    // At idle (ctxPct=0) neither ctxWarn nor ctxDanger should appear.
    const className = await ctxSeg.getAttribute('class') ?? ''
    expect(className).not.toMatch(/ctxWarn|ctxDanger/)

    await page.screenshot({ path: path.join(DIR, `status-idle-${mode}.png`) })
  })
}
