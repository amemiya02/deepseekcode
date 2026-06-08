import { readFileSync, readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import { describe, it, expect } from 'vitest'

/**
 * Interaction inventory snapshot (test strategy layer L1b).
 *
 * Goal: make "we cover every interaction" enforceable rather than aspirational.
 * This enumerates every JSX event-handler prop (`onClick`, `onChange`,
 * `onSubmit`, ...) across every component and pins the result to a committed
 * snapshot. Adding a new interactive control (a new settings toggle, a new
 * chat-area button) changes the inventory, which fails this test until someone
 * runs `npm test -- -u` to accept it — the forcing function that makes a new
 * interaction get noticed (and, by team convention, get a behaviour test +
 * the no-op guard above).
 *
 * The snapshot lives at src/lib/__snapshots__/interaction-inventory.json.
 * Regenerate intentionally: `npx vitest run -u src/lib/interaction-inventory.test.ts`.
 */

const thisDir = path.dirname(fileURLToPath(import.meta.url))
const srcRoot = path.resolve(thisDir, '..') // web/src

function tsxFiles(dir: string): string[] {
  const out: string[] = []
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) out.push(...tsxFiles(p))
    else if (e.name.endsWith('.tsx') && !e.name.includes('.test.')) out.push(p)
  }
  return out
}

interface Entry {
  file: string
  handlers: string[]
}

function buildInventory(): Entry[] {
  const re = /\bon[A-Z][A-Za-z]*(?==\{)/g
  const entries: Entry[] = []
  for (const file of tsxFiles(srcRoot).sort()) {
    const src = readFileSync(file, 'utf8')
    const handlers = new Set<string>()
    for (const m of src.matchAll(re)) handlers.add(m[0])
    if (handlers.size === 0) continue
    entries.push({ file: path.relative(srcRoot, file), handlers: [...handlers].sort() })
  }
  return entries
}

describe('interaction inventory', () => {
  it('matches the committed inventory snapshot', async () => {
    const inventory = buildInventory()
    // Sanity: a broken walker that finds nothing must not silently "match".
    expect(inventory.length, 'components with interactions found').toBeGreaterThan(5)
    await expect(`${JSON.stringify(inventory, null, 2)}\n`).toMatchFileSnapshot(
      path.join(thisDir, '__snapshots__', 'interaction-inventory.json'),
    )
  })
})
