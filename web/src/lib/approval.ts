// Approval helpers: classify a permission request and synthesize a diff patch the
// DiffView can render, so EDIT approvals become an inline diff gate (spec §5) instead
// of a generic JSON modal. Pure + unit-testable; no DOM, no network.
import type { PermissionRequest } from './api'

// The file-mutating tools whose approval renders as an inline diff gate. Everything
// else (bash/command/read_file/destructive) keeps the PermissionModal.
export const EDIT_TOOLS = ['edit_file', 'write_file', 'apply_patch'] as const

export function isEditApproval(req: PermissionRequest | null | undefined): boolean {
  return req != null && (EDIT_TOOLS as readonly string[]).includes(req.tool)
}

// Split into lines without a spurious trailing empty element from a final newline.
function lines(s: string): string[] {
  return s.replace(/\n$/, '').split('\n')
}

// buildApprovalPatch turns a pending edit request into a { path, patch } pair where
// `patch` is a minimal unified-diff string parseHunks() understands. We show the
// targeted region: edit_file old→deletions / new→additions; write_file content as
// additions; apply_patch its envelope text under a synthetic hunk header.
export function buildApprovalPatch(req: PermissionRequest): { path: string; patch: string } {
  const a = req.args ?? {}
  if (req.tool === 'edit_file') {
    const path = String(a.path ?? '')
    const body = [
      ...lines(String(a.old_string ?? '')).map((l) => `-${l}`),
      ...lines(String(a.new_string ?? '')).map((l) => `+${l}`),
    ].join('\n')
    return { path, patch: `@@ ${path || 'edit'} @@\n${body}` }
  }
  if (req.tool === 'write_file') {
    const path = String(a.path ?? '')
    const body = lines(String(a.content ?? '')).map((l) => `+${l}`).join('\n')
    return { path, patch: `@@ ${path || 'write'} @@\n${body}` }
  }
  // apply_patch: codex-style envelope in `patchText`. Extract the first file path if
  // present; prepend a hunk header so the +/- body renders as a diff.
  const text = String(a.patchText ?? '')
  const m = /\*\*\* (?:Update|Add|Delete) File: (.+)/.exec(text)
  const path = m ? m[1].trim() : 'patch'
  return { path, patch: `@@ ${path} @@\n${text}` }
}
