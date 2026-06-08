import { readFileSync, readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import { describe, it, expect } from 'vitest'

/**
 * FE↔BE contract gate (test strategy layer L2).
 *
 * The web client talks to the Go gateway purely over `/v1/*` HTTP endpoints.
 * A button can render, pass its unit test, AND be mounted — yet still do
 * nothing because it calls an endpoint the backend never registered (the 404
 * is swallowed by a `.catch(() => {})`). Unit tests can't see that, and the
 * Playwright suite runs against `mockGateway.ts` whose mock happily answers,
 * masking the gap.
 *
 * This gate closes that hole deterministically: it extracts every `/v1/*`
 * literal the frontend fetches from `api.ts`, extracts every route the Go
 * gateway registers via `mux.HandleFunc(...)`, and asserts the frontend set is
 * a subset of the backend set (honouring net/http ServeMux subtree semantics,
 * where a pattern ending in `/` matches any deeper path).
 */

const thisDir = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(thisDir, '../../..')
const apiPath = path.join(thisDir, 'api.ts')
const gatewayDir = path.join(repoRoot, 'internal', 'gateway')

/** Pull `/v1/...` string/template literals out of the frontend API client. */
function frontendEndpoints(): Set<string> {
  const src = readFileSync(apiPath, 'utf8')
  const out = new Set<string>()
  // A quote/backtick, then /v1/, then the path up to the first delimiter:
  // closing quote, `?` (query), `$`/`{` (template interpolation), whitespace, `)`, `,`, `;`.
  const re = /['"`]\/v1\/[^'"`?${)\s,;]*/g
  for (const m of src.matchAll(re)) {
    out.add(m[0].slice(1)) // drop the leading quote char
  }
  return out
}

/** Pull `/v1/...` patterns out of every gateway HandleFunc registration. */
function backendPatterns(): string[] {
  const out: string[] = []
  const re = /HandleFunc\(\s*"(\/v1\/[^"]*)"/g
  for (const file of readdirSync(gatewayDir)) {
    if (!file.endsWith('.go') || file.endsWith('_test.go')) continue
    const src = readFileSync(path.join(gatewayDir, file), 'utf8')
    for (const m of src.matchAll(re)) out.push(m[1])
  }
  return out
}

const fe = frontendEndpoints()
const be = backendPatterns()
const beExact = new Set(be)
const bePrefixes = be.filter((p) => p.endsWith('/')) // ServeMux subtree handlers

/** ServeMux match: exact, or covered by a registered subtree (`/foo/`) prefix. */
function isCovered(endpoint: string): boolean {
  if (beExact.has(endpoint)) return true
  return bePrefixes.some((p) => endpoint.startsWith(p))
}

describe('FE↔BE /v1 contract', () => {
  // Guard against a broken extractor silently passing (empty ⊆ anything).
  it('extracts a meaningful number of endpoints from both sides', () => {
    expect(fe.size, 'frontend /v1 endpoints found').toBeGreaterThan(8)
    expect(be.length, 'backend /v1 routes found').toBeGreaterThan(20)
  })

  it('every /v1 endpoint the frontend calls is registered in the Go gateway', () => {
    const violations = [...fe].filter((e) => !isCovered(e)).sort()
    expect(
      violations,
      violations.length
        ? `Frontend calls these /v1 endpoints with no Go handler (button will 404 silently):\n  ${violations.join('\n  ')}`
        : '',
    ).toEqual([])
  })
})
