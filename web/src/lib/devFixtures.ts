import type { TranscriptItem } from './transcript'
import type { SlashCommand } from '../components/SlashMenu'
import type { ModelInfo, Session, PermissionRequest } from './api'

export const chatFixture: TranscriptItem[] = [
  { type: 'user', text: 'Refactor the auth module and show me the diff.', pills: ['auth.ts', 'src/'] },
  { type: 'thinking', text: 'The auth module mixes parsing and validation. I will split them and add a guard.', startedAt: 0, endedAt: 3200 },
  { type: 'tool', id: 't1', name: 'read_file', args: { path: 'src/auth.ts' }, readOnly: true, status: 'ok', result: 'export function login() { /* … */ }' },
  { type: 'assistant', streaming: false, text: [
    '## Plan',
    '',
    'I split `auth.ts` into `parse` and `validate`. Key change:',
    '',
    '- extract `validateToken`',
    '- keep the public `login()` signature',
    '',
    'Here is the new validator:',
    '',
    '```ts',
    'export function validateToken(raw: string): Token {',
    '  const t = parseToken(raw)',
    '  if (!t.exp || t.exp < Date.now()) throw new Error("expired")',
    '  return t',
    '}',
    '```',
    '',
    'Inline like `parseToken()` stays in `parse.ts`.',
  ].join('\n') },
  { type: 'tool', id: 't2', name: 'bash', args: { command: 'npm test' }, readOnly: false, status: 'ok', result: '12 passing\n0 failing' },
  { type: 'tool', id: 't3', name: 'edit_file', args: { path: 'src/auth.ts' }, readOnly: false, status: 'error', result: 'patch did not apply: context mismatch at line 42' },
  { type: 'routing', from: 'dsc-chat', to: 'dsc-reasoner', reason: 'multi-file refactor' },
  { type: 'assistant', streaming: true, text: 'Re-applying the patch against the current file' },
]

export const fixtures: Record<string, TranscriptItem[]> = { chat: chatFixture }

// DEV seam: demo slash commands for composer QA (fixture only — tree-shaken from prod).
export const demoCommands: SlashCommand[] = [
  { name: 'fix', description: 'Fix the selected code or error', kind: 'builtin' },
  { name: 'explain', description: 'Explain the selected code in plain English', kind: 'builtin' },
  { name: 'test', description: 'Generate unit tests for the selected code', kind: 'builtin' },
  { name: 'refactor', description: 'Refactor for readability and performance', kind: 'builtin' },
  { name: 'docs', description: 'Add or update documentation comments', kind: 'builtin' },
  { name: 'commit', description: 'Draft a conventional-commit message', kind: 'skill' },
  { name: 'pr-review', description: 'Summarise and review the open pull request', kind: 'skill' },
  { name: 'search', description: 'Search the codebase with a semantic query', kind: 'mcp', hint: 'codegraph' },
]

// DEV seam: demo models for composer model-switcher QA.
export const demoModels: ModelInfo[] = [
  { id: 'deepseek-chat', label: 'deepseek-chat' },
  { id: 'deepseek-reasoner', label: 'deepseek-reasoner' },
]

// DEV seam: demo sessions for session-rail QA — one each in today (×2), yesterday,
// week, month, older. Pass the current epoch ms so grouping is deterministic.
export function sessionsFixture(now: number): Session[] {
  const h = 3_600_000, d = 86_400_000
  const mk = (id: string, title: string, ago: number, turns: number): Session =>
    ({ id, title, turns, updated_at: now - ago, created_at: now - ago })
  return [
    mk('s1', 'Wire gateway SSE replay buffer', 0.5 * h, 8),
    mk('s2', 'Composer single-bar redesign', 2 * h, 14),
    mk('s3', 'Fix /v1/models active shape', 26 * h, 3),
    mk('s4', 'Tau-bench parity investigation', 4 * d, 21),
    mk('s5', 'Prefix-cache A/B harness', 12 * d, 9),
    mk('s6', 'Initial Wails v3 migration', 40 * d, 33),
    mk('s7', 'Repo bootstrap', 95 * d, 2),
  ]
}

// DEV-only: seed a pending permission so ?fixture=approval renders the inline gate
// and ?fixture=approval-cmd renders the fallback modal. Tree-shaken from production.
export const permissionFixtures: Record<string, PermissionRequest> = {
  approval: { id: 'fx-edit', tool: 'edit_file', args: { path: 'src/parser.ts', old_string: 'function doEverything(src) {\n  /* …38 lines… */\n}', new_string: 'export function parse(src) {\n  return build(tokenize(src))\n}' }, options: [] },
  'approval-cmd': { id: 'fx-cmd', tool: 'bash', args: { command: 'rm -rf build/' }, options: [] },
}
