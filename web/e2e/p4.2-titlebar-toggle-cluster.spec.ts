/**
 * P4.2 Title-Bar Toggle Cluster — Playwright visual QA
 *
 * Verifies that:
 *   - The chat area has NO floating collapse buttons (they moved to the title bar).
 *   - The title-bar segmented pair (rail-toggle ┃ panel-toggle) is visible in light + dark.
 *   - Clicking the rail toggle collapses / expands the sessions rail
 *     (data-sessions-collapsed flips on app-shell).
 *   - Clicking the panel toggle opens / closes the review pane
 *     (data-workspace-collapsed flips on app-shell).
 *
 * Harness:
 *   - No webServer block — requires `npm run dev` on :5173
 *   - Theme seeded via localStorage addInitScript (key: dsc.theme)
 *   - /v1/models mocked so the composer doesn't hang on fetch
 *   - Screenshots written to test-results/p4.2/
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/p4.2-titlebar-toggle-cluster.spec.ts
 */

import { expect, Page, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const DIR = path.resolve(__dirname, '..', 'test-results', 'p4.2')
if (!fs.existsSync(DIR)) fs.mkdirSync(DIR, { recursive: true })

async function setup(page: Page, mode: 'light' | 'dark') {
  await page.addInitScript(
    (m: string) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })),
    mode,
  )
  await page.route('**/v1/models', (r) =>
    r.fulfill({
      json: { models: ['deepseek-chat', 'deepseek-reasoner'], active: 'deepseek-chat', effort: 'high' },
    }),
  )
  await page.route('**/v1/sessions', (r) => r.fulfill({ json: { sessions: [] } }))
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/')
  await page.waitForSelector('[data-testid="app-shell"]', { timeout: 15_000 })
  await page.waitForTimeout(300)
}

for (const mode of ['light', 'dark'] as const) {
  test(`titlebar-toggles ${mode} — no floating collapse buttons in chat area`, async ({ page }) => {
    await setup(page, mode)

    // The old floating collapse tabs must NOT exist anywhere in the page
    expect(await page.$('[data-testid="collapse-sessions"]')).not.toBeNull() // exists in title bar
    // But it must be inside the title bar / header, NOT inside <main>
    const inMain = await page.$('main [data-testid="collapse-sessions"]')
    expect(inMain).toBeNull()
    const inMainWorkspace = await page.$('main [data-testid="collapse-workspace"]')
    expect(inMainWorkspace).toBeNull()

    await page.screenshot({ path: path.join(DIR, `titlebar-${mode}.png`) })
  })

  test(`titlebar-toggles ${mode} — segmented pair visible in title bar`, async ({ page }) => {
    await setup(page, mode)

    const titlebar = page.locator('header.titlebar, header[class*="titlebar"]').first()
    await expect(titlebar).toBeVisible()

    const railBtn = page.getByTestId('collapse-sessions')
    const panelBtn = page.getByTestId('collapse-workspace')
    await expect(railBtn).toBeVisible()
    await expect(panelBtn).toBeVisible()

    // Both buttons must be descendants of the header (title bar)
    const railInHeader = await page.$('header [data-testid="collapse-sessions"]')
    expect(railInHeader).not.toBeNull()
    const panelInHeader = await page.$('header [data-testid="collapse-workspace"]')
    expect(panelInHeader).not.toBeNull()

    await titlebar.screenshot({ path: path.join(DIR, `titlebar-pair-${mode}.png`) })
  })

  test(`titlebar-toggles ${mode} — rail toggle collapses and re-expands sessions`, async ({ page }) => {
    await setup(page, mode)

    const shell = page.getByTestId('app-shell')

    // Initial state: sessions rail is expanded
    await expect(shell).toHaveAttribute('data-sessions-collapsed', 'false')

    // Click rail toggle → rail collapses
    await page.getByTestId('collapse-sessions').click()
    await page.waitForTimeout(150)
    await expect(shell).toHaveAttribute('data-sessions-collapsed', 'true')

    await page.screenshot({ path: path.join(DIR, `rail-collapsed-${mode}.png`) })

    // Click again → rail re-expands
    await page.getByTestId('collapse-sessions').click()
    await page.waitForTimeout(150)
    await expect(shell).toHaveAttribute('data-sessions-collapsed', 'false')

    await page.screenshot({ path: path.join(DIR, `rail-expanded-${mode}.png`) })
  })

  test(`titlebar-toggles ${mode} — panel toggle opens and closes review pane`, async ({ page }) => {
    await setup(page, mode)

    const shell = page.getByTestId('app-shell')

    // Initial state without changes: workspace pane is closed (auto + no changes)
    await expect(shell).toHaveAttribute('data-workspace-collapsed', 'true')

    // Click panel toggle → review pane pins open
    await page.getByTestId('collapse-workspace').click()
    await page.waitForTimeout(150)
    await expect(shell).toHaveAttribute('data-workspace-collapsed', 'false')

    await page.screenshot({ path: path.join(DIR, `panel-open-${mode}.png`) })

    // Click again → review pane closes
    await page.getByTestId('collapse-workspace').click()
    await page.waitForTimeout(150)
    await expect(shell).toHaveAttribute('data-workspace-collapsed', 'true')

    await page.screenshot({ path: path.join(DIR, `panel-closed-${mode}.png`) })
  })
}
