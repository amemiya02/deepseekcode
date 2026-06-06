/**
 * P3.3b Mid-Turn Steering — Playwright behavioral QA
 *
 * Verifies Codex-style mid-turn steering end-to-end:
 *   1. Submit a first prompt → UI enters streaming state (Stop button visible).
 *   2. Type a second message while streaming → steer hint visible.
 *   3. Press Enter → POST /v1/steer fired with the text.
 *   4. No second POST /v1/prompt fired.
 *
 * Harness mirrors p3.3-adaptive-review.spec.ts:
 *   - No webServer block — requires `npm run dev` on :5173
 *   - Theme seeded via localStorage addInitScript (key: dsc.theme)
 *   - /v1/prompt mock returns a session_id immediately
 *   - /v1/events SSE stream emits a couple of text deltas and then stalls
 *     (never sends turn_done) so the UI stays in streaming state
 *   - /v1/steer mock records the request body and returns 200
 *   - Screenshots written to test-results/p3.3b/
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/p3.3b-mid-turn-steering.spec.ts
 */

import { expect, Page, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const DIR = path.resolve(__dirname, '..', 'test-results', 'p3.3b')
if (!fs.existsSync(DIR)) fs.mkdirSync(DIR, { recursive: true })

const SESSION_ID = 'sess-steer-1'

/**
 * Build a minimal SSE stream body that emits two message_delta events and then
 * stalls forever (never sends turn_done), keeping the UI in streaming state.
 */
function sseBody(): string {
  const delta1 = JSON.stringify({ type: 'message_delta', text: 'Working' })
  const delta2 = JSON.stringify({ type: 'message_delta', text: ' on it…' })
  return `data: ${delta1}\n\ndata: ${delta2}\n\n`
}

async function setup(page: Page, mode: string) {
  // Seed theme via localStorage before page load.
  await page.addInitScript(
    (m) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })),
    mode,
  )

  // Standard shell endpoints.
  await page.route('**/v1/models', (r) =>
    r.fulfill({
      json: {
        models: ['deepseek-chat', 'deepseek-reasoner'],
        active: 'deepseek-chat',
        effort: 'high',
      },
    }),
  )
  await page.route('**/v1/sessions', (r) => r.fulfill({ json: { sessions: [] } }))
  await page.route('**/v1/changed', (r) => r.fulfill({ json: { entries: [] } }))
  await page.route('**/v1/telemetry**', (r) => r.fulfill({ status: 200, body: '' }))

  // POST /v1/prompt → return session_id immediately (no second run should occur).
  await page.route('**/v1/prompt', (r) =>
    r.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ request_id: 'req-1', session_id: SESSION_ID }),
    }),
  )

  // GET /v1/events — emit two deltas then stall (no turn_done) so streaming stays true.
  await page.route(`**/v1/events**`, (r) =>
    r.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      headers: {
        'Cache-Control': 'no-cache',
        Connection: 'keep-alive',
      },
      body: sseBody(),
    }),
  )

  // POST /v1/steer — always 200 (recording is done per-test via request interception below).
  await page.route('**/v1/steer', (r) => r.fulfill({ status: 200, body: '' }))

  // Other session-related endpoints that may be called during onboarding/update checks.
  await page.route('**/v1/onboarding**', (r) =>
    r.fulfill({ json: { needsOnboarding: false } }),
  )
  await page.route('**/v1/update**', (r) =>
    r.fulfill({ json: { updateAvailable: false } }),
  )

  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/')
  await page.waitForSelector('[data-testid="app-shell"]', { timeout: 15_000 })
  await page.waitForTimeout(300)
}

for (const mode of ['light', 'dark'] as const) {
  test(`mid-turn steering ${mode} — steer fires instead of new prompt`, async ({ page }) => {
    await setup(page, mode)

    // Capture every POST /v1/prompt and /v1/steer for later assertions.
    const promptRequests: string[] = []
    const steerBodies: Array<{ session_id: string; prompt: string }> = []

    page.on('request', (req) => {
      const url = req.url()
      if (req.method() === 'POST' && url.includes('/v1/prompt')) {
        promptRequests.push(url)
      }
      if (req.method() === 'POST' && url.includes('/v1/steer')) {
        try {
          steerBodies.push(JSON.parse(req.postData() ?? '{}'))
        } catch {
          // ignore parse errors
        }
      }
    })

    // Step 1: Submit first prompt.
    const composer = page.getByTestId('composer-input')
    await composer.click()
    await composer.fill('Build the parser module')
    await composer.press('Enter')

    // Step 2: Wait for streaming state — Stop button (Square icon) must appear.
    // send-stop shows as Stop when streaming=true.
    await page.waitForTimeout(600)
    const sendStop = page.getByTestId('send-stop')
    // The button exists; when streaming it carries the stop role (aria-label='Stop').
    await expect(sendStop).toBeVisible({ timeout: 5_000 })

    // Confirm we're streaming: the stop button should have aria-label "Stop".
    await expect(sendStop).toHaveAttribute('aria-label', 'Stop', { timeout: 5_000 })

    // Step 3: Type a second message while streaming.
    await composer.click()
    await composer.fill('focus on the tokenizer first')

    // Step 4: Steer hint must be visible (streaming=true AND text.trim().length > 0).
    await expect(page.getByTestId('composer-steer-hint')).toBeVisible({ timeout: 3_000 })

    // Screenshot: hint visible, stop button present.
    await page.screenshot({ path: path.join(DIR, `steer-hint-visible-${mode}.png`) })

    // Step 5: Press Enter to steer.
    await composer.press('Enter')
    await page.waitForTimeout(400)

    // Step 6: Assert POST /v1/steer fired with the second message's text.
    expect(steerBodies.length).toBeGreaterThanOrEqual(1)
    const steer = steerBodies[steerBodies.length - 1]
    expect(steer.session_id).toBe(SESSION_ID)
    expect(steer.prompt).toBe('focus on the tokenizer first')

    // Step 7: Assert NO second POST /v1/prompt was fired.
    // promptRequests has exactly one entry (the initial submit); no second.
    expect(promptRequests.length).toBe(1)

    // Screenshot: after steer, user bubble echoed, still streaming.
    await page.screenshot({ path: path.join(DIR, `steer-sent-${mode}.png`) })
  })

  test(`mid-turn steering ${mode} — steer hint absent when not streaming`, async ({ page }) => {
    await setup(page, mode)

    // Before any submit, streaming=false; type something.
    const composer = page.getByTestId('composer-input')
    await composer.click()
    await composer.fill('some text before any turn')

    // Steer hint must NOT be visible when not streaming.
    expect(await page.$('[data-testid="composer-steer-hint"]')).toBeNull()

    await page.screenshot({ path: path.join(DIR, `steer-hint-absent-${mode}.png`) })
  })
}
