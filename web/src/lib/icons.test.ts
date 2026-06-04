import { describe, it, expect } from 'vitest'
import * as icons from './icons'

const EXPECTED = [
  'IconSearch', 'IconPlus', 'IconSend', 'IconStop', 'IconSettings',
  'IconSun', 'IconMoon', 'IconPalette', 'IconChevronRight', 'IconChevronDown',
  'IconX', 'IconFile', 'IconFolder', 'IconGitBranch', 'IconCommand',
  'IconAlertTriangle', 'IconCheck', 'IconCopy', 'IconRefresh', 'IconPanelLeft',
  'IconPanelRight', 'IconActivity', 'IconCoins', 'IconDatabase',
]

describe('icons module', () => {
  it('exports every curated icon as a defined component', () => {
    for (const name of EXPECTED) {
      expect((icons as Record<string, unknown>)[name], `${name} missing`).toBeTruthy()
    }
  })

  it('exports no emoji or raw-string icons (all are functions/objects)', () => {
    for (const name of EXPECTED) {
      const v = (icons as Record<string, unknown>)[name]
      expect(typeof v === 'function' || typeof v === 'object').toBe(true)
    }
  })
})
