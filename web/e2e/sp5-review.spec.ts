/**
 * SP5 Review Hero + TelemetryStrip — Playwright visual QA
 *
 * Tests the Codex-grade right-pane review surface across light/dark modes:
 *   - review-rest  — changed-files list ‖ diff hero after selecting the first file
 *   - telemetry-collapsed — compact TelemetryStrip at rest
 *   - telemetry-expanded  — after clicking telemetry-toggle (Cockpit cards visible)
 *
 * Structural assertions:
 *   - review-hero present
 *   - at least one diff-hunk visible after file selection
 *   - NO ws-tab-cockpit / ws-tab-workspace elements (old tab structure gone)
 *   - telemetry-toggle toggles telemetry-expanded visibility
 *
 * Mirrors the harness in sp4-rail.spec.ts:
 *   - No webServer block — requires `npm run dev` on :5173
 *   - Theme seeded via localStorage addInitScript (key: dsc.theme)
 *   - /v1/models + /v1/sessions + /v1/changed + /v1/diff + /v1/file route-mocked
 *   - Screenshots written to test-results/sp5/
 *
 * Run:
 *   npm run dev &   # in web/
 *   npx playwright test e2e/sp5-review.spec.ts
 */

import { expect, Page, test } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const DIR = path.resolve(__dirname, '..', 'test-results', 'sp5')
if (!fs.existsSync(DIR)) fs.mkdirSync(DIR, { recursive: true })

// Realistic three-entry changed-files list covering all interesting status codes.
const CHANGED_ENTRIES = [
  { path: 'internal/gateway/workspace.go', status: 'M', deleted: false },
  { path: 'web/src/components/ReviewPanel.tsx', status: 'A', deleted: false },
  { path: 'README.md', status: 'M', deleted: false },
]

// A realistic multi-hunk unified diff patch (two hunks, +/- lines, @@ headers).
const SAMPLE_PATCH = `--- a/internal/gateway/workspace.go
+++ b/internal/gateway/workspace.go
@@ -12,6 +12,8 @@ import (
 	"net/http"
 	"os"
 	"os/exec"
+	"bytes"
+	"strings"
 )

 // handleChanged lists git working-tree changes via \`git status --porcelain\`.
@@ -98,0 +101,22 @@ func (h *Handler) handleChanged(w http.ResponseWriter, r *http.Request) {
+
+// handleDiff implements GET /v1/diff?path= — the unified working-tree diff for a
+// single path via \`git diff --no-color HEAD -- <path>\` (staged + unstaged vs the
+// last commit). A non-git root, missing HEAD, or git error yields an empty patch
+// (200), never a 500 — the SPA shows "no diff" / falls back to a full-file view.
+func (h *Handler) handleDiff(w http.ResponseWriter, r *http.Request) {
+	if r.Method != http.MethodGet {
+		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
+		return
+	}
+	rel := r.URL.Query().Get("path")
+	if rel == "" {
+		http.Error(w, "missing path", http.StatusBadRequest)
+		return
+	}
+	if _, err := resolveInRoot(h.root, rel); err != nil {
+		http.Error(w, "bad path: "+err.Error(), http.StatusBadRequest)
+		return
+	}
+	patch := ""
+	if h.root != "" {
+		cmd := exec.CommandContext(r.Context(), "git", "-C", h.root, "diff", "--no-color", "HEAD", "--", rel)
+		var buf bytes.Buffer
+		cmd.Stdout = &buf
+		if err := cmd.Run(); err == nil {
+			patch = buf.String()
+		}
+	}
+	writeJSON(w, map[string]any{"path": filepath.ToSlash(rel), "patch": patch})
+}`

// Small file content payload for the fallback code-viewer case.
const FILE_CONTENT = {
  path: 'README.md',
  content: '# DeepSeek Code\n\nAn industrial-grade coding assistant.\n',
  binary: false,
  truncated: false,
}

async function setup(page: Page, mode: string) {
  // Seed theme via localStorage before page load.
  await page.addInitScript((m) => localStorage.setItem('dsc.theme', JSON.stringify({ mode: m })), mode)

  // Route-mock all gateway endpoints the ReviewPanel + TelemetryStrip consume.
  await page.route('**/v1/models', (r) =>
    r.fulfill({
      json: { models: ['deepseek-chat', 'deepseek-reasoner'], active: 'deepseek-chat', effort: 'high' },
    })
  )
  await page.route('**/v1/sessions', (r) =>
    r.fulfill({ json: { sessions: [] } })
  )
  await page.route('**/v1/changed', (r) =>
    r.fulfill({ json: { entries: CHANGED_ENTRIES } })
  )
  // Diff: return the realistic multi-hunk patch for any path.
  await page.route('**/v1/diff**', (r) => {
    const url = new URL(r.request().url())
    const filePath = url.searchParams.get('path') ?? 'unknown'
    r.fulfill({ json: { path: filePath, patch: SAMPLE_PATCH } })
  })
  // File: return the small content payload for any path.
  await page.route('**/v1/file**', (r) =>
    r.fulfill({ json: FILE_CONTENT })
  )
  // Cockpit telemetry SSE endpoint — return empty 200 so the strip doesn't error.
  await page.route('**/v1/telemetry**', (r) =>
    r.fulfill({ status: 200, body: '' })
  )

  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/')

  // Wait for the right pane review hero to mount.
  await page.waitForSelector('[data-testid="review-hero"]', { timeout: 15_000 })
  await page.waitForSelector('[data-testid="telemetry-toggle"]', { timeout: 10_000 })
  await page.waitForTimeout(400)
}

for (const mode of ['light', 'dark'] as const) {
  test(`review ${mode} rest — list ‖ diff after file selection`, async ({ page }) => {
    await setup(page, mode)

    // Structural: review-hero present, old tabs absent.
    await expect(page.getByTestId('review-hero')).toBeVisible()
    expect(await page.$('[data-testid="ws-tab-cockpit"]')).toBeNull()
    expect(await page.$('[data-testid="ws-tab-workspace"]')).toBeNull()

    // Initially shows placeholder (no file selected).
    await expect(page.getByTestId('review-placeholder')).toBeVisible()

    // Click the first changed-file row to select it — triggers fetchDiff.
    const firstRow = page.getByTestId('change-status').first()
    await firstRow.click()
    await page.waitForTimeout(500)

    // After selection: at least one diff-hunk must be visible.
    const hunks = page.locator('[data-testid="diff-hunk"]')
    await expect(hunks.first()).toBeVisible({ timeout: 5_000 })

    // Screenshot the full workspace zone (list ‖ diff hero).
    const zone = page.getByTestId('zone-workspace')
    await zone.screenshot({ path: path.join(DIR, `review-rest-${mode}.png`) })
  })

  test(`review ${mode} telemetry-collapsed — compact strip at rest`, async ({ page }) => {
    await setup(page, mode)

    // Compact bar is always visible.
    await expect(page.getByTestId('telemetry-toggle')).toBeVisible()
    await expect(page.getByTestId('telemetry-compact')).toBeVisible()

    // Expanded panel must NOT be visible yet.
    expect(await page.$('[data-testid="telemetry-expanded"]')).toBeNull()

    // Screenshot the workspace zone showing the collapsed strip.
    const zone = page.getByTestId('zone-workspace')
    await zone.screenshot({ path: path.join(DIR, `telemetry-collapsed-${mode}.png`) })
  })

  test(`review ${mode} telemetry-expanded — Cockpit cards after toggle`, async ({ page }) => {
    await setup(page, mode)

    // Click the toggle to expand.
    await page.getByTestId('telemetry-toggle').click()
    await page.waitForTimeout(300)

    // Expanded panel must now be visible.
    await expect(page.getByTestId('telemetry-expanded')).toBeVisible()

    // Screenshot the workspace zone with Cockpit cards visible.
    const zone = page.getByTestId('zone-workspace')
    await zone.screenshot({ path: path.join(DIR, `telemetry-expanded-${mode}.png`) })

    // Click toggle again — expanded panel should disappear.
    await page.getByTestId('telemetry-toggle').click()
    await page.waitForTimeout(200)
    expect(await page.$('[data-testid="telemetry-expanded"]')).toBeNull()
  })
}
