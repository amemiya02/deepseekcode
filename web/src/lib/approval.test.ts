import { describe, it, expect } from 'vitest'
import { isEditApproval, buildApprovalPatch, EDIT_TOOLS } from './approval'
import type { PermissionRequest } from './api'

const mk = (tool: string, args: Record<string, unknown>): PermissionRequest => ({ id: '1', tool, args, options: [] })

describe('isEditApproval', () => {
  it('is true for the edit tools', () => {
    for (const tool of EDIT_TOOLS) expect(isEditApproval(mk(tool, {}))).toBe(true)
  })
  it('is false for non-edit tools and null', () => {
    expect(isEditApproval(mk('bash', { command: 'ls' }))).toBe(false)
    expect(isEditApproval(mk('read_file', { path: 'a.ts' }))).toBe(false)
    expect(isEditApproval(null)).toBe(false)
  })
})

describe('buildApprovalPatch', () => {
  it('edit_file → old as deletions, new as additions, with a hunk header', () => {
    const { path, patch } = buildApprovalPatch(mk('edit_file', { path: 'a.ts', old_string: 'const x = 1', new_string: 'const x = 2' }))
    expect(path).toBe('a.ts')
    expect(patch.split('\n')[0]).toMatch(/^@@/)
    expect(patch).toContain('-const x = 1')
    expect(patch).toContain('+const x = 2')
  })
  it('write_file → content as additions', () => {
    const { path, patch } = buildApprovalPatch(mk('write_file', { path: 'b.ts', content: 'line1\nline2' }))
    expect(path).toBe('b.ts')
    expect(patch).toContain('+line1')
    expect(patch).toContain('+line2')
  })
  it('apply_patch → extracts the file path and wraps the envelope', () => {
    const { path, patch } = buildApprovalPatch(mk('apply_patch', { patchText: '*** Begin Patch\n*** Update File: c.ts\n-old\n+new\n*** End Patch' }))
    expect(path).toBe('c.ts')
    expect(patch).toContain('+new')
  })
})
