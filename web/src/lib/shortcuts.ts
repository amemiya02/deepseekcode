export interface ShortcutSpec {
  key: string
  meta?: boolean // Cmd on mac OR Ctrl on win/linux
  shift?: boolean
  alt?: boolean
}

export function matchShortcut(e: KeyboardEvent, spec: ShortcutSpec): boolean {
  if (e.key.toLowerCase() !== spec.key.toLowerCase()) return false
  const wantMeta = spec.meta ?? false
  const gotMeta = e.metaKey || e.ctrlKey
  if (wantMeta !== gotMeta) return false
  if ((spec.shift ?? false) !== e.shiftKey) return false
  if ((spec.alt ?? false) !== e.altKey) return false
  return true
}

export function isCmdK(e: KeyboardEvent): boolean {
  return matchShortcut(e, { key: 'k', meta: true })
}
