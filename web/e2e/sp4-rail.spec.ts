/**
 * SP4 Session Rail — Playwright visual QA
 *
 * Tests the Codex-grade session rail across light/dark modes:
 *   - rest state — populated, grouped (full rail region)
 *   - search state — after typing a query that filters to a subset
 *   - hover state — hover a session-item so rename/delete controls show
 *   - empty state — no sessions loaded (empty hint visible)
 *
 * Structural assertions (run once per mode):
 *   - session-new visible
 *   - session-count shows the right total (7 sessions)
 *   - NO session-history element exists
 *   - NO history-drawer element exists
 *
 * Mirrors the harness in sp3-composer.spec.ts:
 *   - No webServer block — requires `npm run dev` on :5173
 *   - Theme seeded via localStorage addInitScript (key: dsc.theme)
 *   - /v1/models + /v1/sessions route-mocked so fixture works without a real gateway
 *   - /v1/sessions returns 7 deterministic sessions spanning all 5 day-buckets
 *   - Screenshots written to test-results/sp4/
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/sp4-rail.spec.ts
 */

import { expect, Page, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const DIR = path.resolve(__dirname, '..', 'test-results', 'sp4')
if (!fs.existsSync(DIR)) fs.mkdirSync(DIR, { recursive: true })

/** Build 7 demo sessions spanning today(×2), yesterday, week, month, older.
 *  Mirrors sessionsFixture() in devFixtures.ts but runs in Node (no import.meta.env). */
function makeSessions() {
  const now = Date.now()
  const h = 3_600_000, d = 86_400_000
  const mk = (id: string, title: string, ago: number, turns: number) => ({
    id, title, turns, updated_at: now - ago, created_at: now - ago,
  })
  return [
    mk('s1', 'Wire gateway SSE replay buffer', 0.5 * h, 8),
    mk('s2', 'Composer single-bar redesign', 2 * h, 14),
    mk('s3', 'Fix /v1/models active shape', 26 * h, 3),
    mk('s4', 'Tau-bench parity investigation', 4 * d, 21),
    mk('s5', 'Prefix-cache A/B harness', 12 * d, 9),
    mk('s6', 'Initial Wails v3 migration', 40 * d, 33),
    mk('s7', 'Repo bootstrap', 95 * d, 2),
  ]
}

const SESSIONS = makeSessions()

async function setup(page: Page, mode: string) {
  await page.addInitScript((m) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })), mode)
  await page.route('**/v1/models', (r) =>
    r.fulfill({
      json: { models: ['deepseek-chat', 'deepseek-reasoner'], active: 'deepseek-chat', effort: 'high' },
    })
  )
  // Mock /v1/sessions so the store's load() call fills the rail with deterministic data.
  // The fixture seam (?fixture=sessions) also seeds via useSessionStore.setState but the
  // mount loadSessions() call races it. Mocking the API endpoint ensures load() always
  // wins with the right sessions and session-count footer appears reliably.
  await page.route('**/v1/sessions', (r) =>
    r.fulfill({ json: { sessions: SESSIONS } })
  )
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/?fixture=sessions')
  // Wait for the session rail and for sessions to load (7 sessions → footer appears)
  await page.waitForSelector('[data-testid="session-new"]', { timeout: 15_000 })
  await page.waitForSelector('[data-testid="session-count"]', { timeout: 10_000 })
  await page.waitForTimeout(300)
}

async function setupEmpty(page: Page, mode: string) {
  await page.addInitScript((m) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })), mode)
  await page.route('**/v1/models', (r) =>
    r.fulfill({
      json: { models: ['deepseek-chat', 'deepseek-reasoner'], active: 'deepseek-chat', effort: 'high' },
    })
  )
  // Return empty sessions list from the API so rail shows empty state
  await page.route('**/v1/sessions', (r) =>
    r.fulfill({ json: { sessions: [] } })
  )
  await page.setViewportSize({ width: 1280, height: 900 })
  // Navigate without fixture=sessions so the fixture seam doesn't seed sessions
  await page.goto('/')
  await page.waitForSelector('[data-testid="session-new"]', { timeout: 15_000 })
  // Wait for loading to finish — empty hint should appear once loading is done
  await page.waitForSelector('[data-testid="sessions-empty"]', { timeout: 10_000 })
  await page.waitForTimeout(300)
}

for (const mode of ['light', 'dark'] as const) {
  test(`rail ${mode} rest — populated, grouped`, async ({ page }) => {
    await setup(page, mode)

    // Structural assertions
    await expect(page.getByTestId('session-new')).toBeVisible()
    await expect(page.getByTestId('session-count')).toHaveText('7 sessions')
    expect(await page.$('[data-testid="session-history"]')).toBeNull()
    expect(await page.$('[data-testid="history-drawer"]')).toBeNull()

    // Screenshot the full rail region
    const rail = page.locator('aside').first()
    await rail.screenshot({ path: path.join(DIR, `rail-rest-${mode}.png`) })
  })

  test(`rail ${mode} search — filtered subset`, async ({ page }) => {
    await setup(page, mode)

    // Type a query that matches a subset of sessions
    const searchInput = page.getByTestId('session-search')
    await searchInput.fill('gateway')
    await page.waitForTimeout(150)

    // Screenshot the rail showing filtered results
    const rail = page.locator('aside').first()
    await rail.screenshot({ path: path.join(DIR, `rail-search-${mode}.png`) })
  })

  test(`rail ${mode} hover — rename/delete controls visible`, async ({ page }) => {
    await setup(page, mode)

    // Hover the first session item to reveal rename/delete controls
    const firstItem = page.getByTestId('session-item').first()
    await firstItem.hover()
    await page.waitForTimeout(150)

    // Screenshot the rail with hover state active
    const rail = page.locator('aside').first()
    await rail.screenshot({ path: path.join(DIR, `rail-hover-${mode}.png`) })
  })

  test(`rail ${mode} empty — no sessions hint`, async ({ page }) => {
    await setupEmpty(page, mode)

    // Assert empty state is shown
    await expect(page.getByTestId('sessions-empty')).toBeVisible()
    // session-count footer should not render when there are no sessions
    expect(await page.$('[data-testid="session-count"]')).toBeNull()

    // Screenshot the empty state
    const rail = page.locator('aside').first()
    await rail.screenshot({ path: path.join(DIR, `rail-empty-${mode}.png`) })
  })
}
