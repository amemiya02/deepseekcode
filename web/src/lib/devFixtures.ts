import type { TranscriptItem } from './transcript'
import type { SlashCommand } from '../components/SlashMenu'
import type { ModelInfo } from './api'

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
