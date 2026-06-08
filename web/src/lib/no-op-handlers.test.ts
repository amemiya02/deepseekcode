import { readFileSync, readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import { describe, it, expect } from 'vitest'

/**
 * No-op handler guard (test strategy layer L1a).
 *
 * The bug this prevents: a button that renders and passes its unit test but
 * does nothing because its handler is an inline no-op (`onClick={() => {}}`).
 * That is exactly how the chat-area buttons shipped "frontend only". Unit
 * tests assert the element renders; they don't notice the handler is empty.
 *
 * This guard statically scans every component for empty/no-op JSX event
 * handlers. A handler that is *intentionally* a no-op (rare) must be added to
 * ALLOWLIST with a reason — that makes the debt visible and reviewable instead
 * of silent. Any new, unlisted no-op fails the build.
 */

const thisDir = path.dirname(fileURLToPath(import.meta.url))
const srcRoot = path.resolve(thisDir, '..') // web/src

/** Intentional no-ops, tracked as visible debt. Each entry needs a reason. */
const ALLOWLIST: ReadonlyArray<{ file: string; handler: string; reason: string }> = []

const NOOP_PATTERNS: RegExp[] = [
  // onX={() => {}} | onX={(a, b) => {}}
  /\bon[A-Z][A-Za-z]*=\{\s*\([^)]*\)\s*=>\s*\{\s*\}\s*\}/g,
  // onX={() => undefined} | onX={() => null} | onX={() => void 0}
  /\bon[A-Z][A-Za-z]*=\{\s*\([^)]*\)\s*=>\s*(?:undefined|null|void 0)\s*\}/g,
  // onX={noop} | onX={() => {}}  passed via a named noop
  /\bon[A-Z][A-Za-z]*=\{\s*noop\s*\}/g,
]

function tsxFiles(dir: string): string[] {
  const out: string[] = []
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) out.push(...tsxFiles(p))
    else if (e.name.endsWith('.tsx') && !e.name.includes('.test.')) out.push(p)
  }
  return out
}

interface Finding {
  file: string
  handler: string
  line: number
}

function findNoOps(): Finding[] {
  const findings: Finding[] = []
  for (const file of tsxFiles(srcRoot)) {
    const src = readFileSync(file, 'utf8')
    const rel = path.relative(srcRoot, file)
    for (const re of NOOP_PATTERNS) {
      for (const m of src.matchAll(re)) {
        const handler = m[0].split('=')[0].trim()
        const line = src.slice(0, m.index ?? 0).split('\n').length
        findings.push({ file: rel, handler, line })
      }
    }
  }
  return findings
}

describe('no-op handler guard', () => {
  it('has no empty/no-op JSX event handlers outside the allowlist', () => {
    const allow = new Set(ALLOWLIST.map((a) => `${a.file}::${a.handler}`))
    const offenders = findNoOps().filter((f) => !allow.has(`${f.file}::${f.handler}`))
    expect(
      offenders,
      offenders.length
        ? `Found ${offenders.length} no-op handler(s) (button renders but does nothing). ` +
            `Wire them to real state/API, or add to ALLOWLIST with a reason:\n  ` +
            offenders.map((f) => `${f.file}:${f.line}  ${f.handler}`).join('\n  ')
        : '',
    ).toEqual([])
  })
})
