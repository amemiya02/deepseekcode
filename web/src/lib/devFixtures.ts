import type { TranscriptItem } from './transcript'

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
