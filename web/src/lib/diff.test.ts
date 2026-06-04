import { describe, it, expect } from 'vitest'
import { parseHunks, hunkSides } from './diff'

const PATCH = `@@ -1,3 +1,4 @@
 context line
-removed line
+added line one
+added line two
 trailing context
@@ -10,2 +11,2 @@
-old
+new`

describe('parseHunks', () => {
  it('parses two hunks', () => {
    expect(parseHunks(PATCH)).toHaveLength(2)
  })

  it('classifies context/del/add lines in the first hunk', () => {
    const [h] = parseHunks(PATCH)
    expect(h.lines.map((l) => l.kind)).toEqual(['context', 'del', 'add', 'add', 'context'])
    expect(h.lines[1].text).toBe('removed line')
    expect(h.lines[2].text).toBe('added line one')
  })

  it('captures the header on each hunk', () => {
    const hunks = parseHunks(PATCH)
    expect(hunks[0].header).toBe('@@ -1,3 +1,4 @@')
    expect(hunks[1].header).toBe('@@ -10,2 +11,2 @@')
  })

  it('returns empty for empty input', () => {
    expect(parseHunks('')).toEqual([])
  })
})

describe('hunkSides', () => {
  it('reconstructs original (context+del) and modified (context+add) text', () => {
    const [h] = parseHunks(PATCH)
    const { original, modified } = hunkSides(h)
    expect(original).toBe('context line\nremoved line\ntrailing context')
    expect(modified).toBe('context line\nadded line one\nadded line two\ntrailing context')
  })
})
