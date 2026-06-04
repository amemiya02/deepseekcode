// English UI strings — the canonical bundle. Keys are dotted by area; values may
// contain {placeholders} filled at call time (see lib/i18n.tsx). zh.ts mirrors
// this key set (a RUNTIME test enforces parity — Contract 3 relaxes compile-time).
export const en: Record<string, string> = {
  'app.title': 'DeepSeekCode',
  'app.newSession': 'New session',
  'app.openPalette': 'Open command palette',
  'titlebar.branch': 'Branch',
  'titlebar.theme': 'Theme',
  'titlebar.toggleMode': 'Toggle light/dark',
  'palette.placeholder': 'Search commands, files, sessions…',
  'palette.results': '{n} results',
  'palette.empty': 'No matches',
  'shell.collapseSessions': 'Collapse sessions',
  'shell.collapseWorkspace': 'Collapse workspace',
  'zone.sessions': 'Sessions',
  'zone.conversation': 'Conversation',
  'zone.workspace': 'Workspace',
  'error.title': 'Something went wrong',
  'error.retry': 'Retry',
  'toast.dismiss': 'Dismiss',
}
