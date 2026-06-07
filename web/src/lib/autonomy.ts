export type AutonomyMode = 'ask' | 'auto-edit' | 'plan' | 'yolo'

export const ORDER: AutonomyMode[] = ['ask', 'auto-edit', 'plan', 'yolo']

export interface ModeMeta { label: string; desc: string }

// Defaults are English; UI may localize via t() at render. Keep keys stable.
export const MODES: Record<AutonomyMode, ModeMeta> = {
  ask: { label: 'Ask', desc: 'confirm each edit' },
  'auto-edit': { label: 'Auto-edit', desc: 'apply edits automatically' },
  plan: { label: 'Plan', desc: 'read-only, no writes' },
  yolo: { label: 'Yolo', desc: 'full auto, no prompts' },
}

export function nextMode(m: AutonomyMode): AutonomyMode {
  return ORDER[(ORDER.indexOf(m) + 1) % ORDER.length]
}

// Permission-tier mapping: each PermissionTier value maps to an AutonomyMode
// so the 3-segment control (read-only / workspace-write / full) can drive autonomy.
export type PermissionTier = 'read-only' | 'workspace-write' | 'full'

export const TIER_TO_MODE: Record<PermissionTier, AutonomyMode> = {
  'read-only': 'plan',
  'workspace-write': 'ask',
  full: 'yolo',
}

export const MODE_TO_TIER: Record<AutonomyMode, PermissionTier> = {
  plan: 'read-only',
  ask: 'workspace-write',
  'auto-edit': 'workspace-write',
  yolo: 'full',
}
