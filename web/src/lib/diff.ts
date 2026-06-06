// Pure unified-diff parser: splits a patch into hunks of classified lines and can
// reconstruct each hunk's original/modified sides for a Monaco DiffEditor.
export interface DiffLine { kind: 'context' | 'add' | 'del'; text: string }
export interface Hunk { header: string; lines: DiffLine[] }

export function parseHunks(patch: string): Hunk[] {
  if (!patch.trim()) return []
  const hunks: Hunk[] = []
  let current: Hunk | null = null
  for (const raw of patch.split('\n')) {
    if (raw.startsWith('@@')) {
      current = { header: raw, lines: [] }
      hunks.push(current)
      continue
    }
    if (!current) continue
    if (raw.startsWith('+')) current.lines.push({ kind: 'add', text: raw.slice(1) })
    else if (raw.startsWith('-')) current.lines.push({ kind: 'del', text: raw.slice(1) })
    else current.lines.push({ kind: 'context', text: raw.startsWith(' ') ? raw.slice(1) : raw })
  }
  return hunks
}

// hunkSides reconstructs the before/after text of one hunk (for Monaco's DiffEditor):
// original = context + deletions; modified = context + additions.
export function hunkSides(hunk: Hunk): { original: string; modified: string } {
  const original = hunk.lines.filter((l) => l.kind !== 'add').map((l) => l.text).join('\n')
  const modified = hunk.lines.filter((l) => l.kind !== 'del').map((l) => l.text).join('\n')
  return { original, modified }
}

// countDiffLines totals added/removed lines across all hunks (for the island header).
export function countDiffLines(patch: string): { added: number; removed: number } {
  let added = 0
  let removed = 0
  for (const hunk of parseHunks(patch)) {
    for (const line of hunk.lines) {
      if (line.kind === 'add') added++
      else if (line.kind === 'del') removed++
    }
  }
  return { added, removed }
}
