// Adapted from deepseek-reasonix (MIT) — SettingsPanel.tsx (nav-rail + active tab + section switch).
import { useState } from 'react'
import { X } from 'lucide-react'
import { t } from '../../lib/i18n'
import {
  IconSettings, IconPalette, IconCommand, IconLanguages, IconModel, IconCoins, IconKey,
  IconShield, IconSandbox, IconEditor, IconDatabase, IconDuet, IconExtensions, IconNetwork,
  IconSessions, IconActivity, IconRefresh, IconInfo, type Icon,
} from '../../lib/icons'
import { GeneralSection } from './GeneralSection'
import { AppearanceSection } from './AppearanceSection'
import { ProvidersSection } from './ProvidersSection'
import { NetworkSection } from './NetworkSection'
import { SandboxSection } from './SandboxSection'
import { ExtensionsSection } from './ExtensionsSection'
import { DoctorView } from './DoctorView'
import { UpdatesSection } from './UpdatesSection'
import { KeybindingsSection } from './KeybindingsSection'
import { ModelsSection } from './ModelsSection'
import { BudgetSection } from './BudgetSection'
import { DuetSection } from './DuetSection'
import { PermissionsSection } from './PermissionsSection'
import { EditorSection } from './EditorSection'
import { ContextSection } from './ContextSection'
import { SessionsSection } from './SessionsSection'
import styles from './SettingsWindow.module.css'

export type SettingsGroup = 'personal' | 'modelsCost' | 'coding' | 'integrations' | 'workspace' | 'system'

export interface SettingsSection {
  id: string
  labelKey: string
  fallback: string
  group: SettingsGroup
  icon: Icon
}

// SETTINGS_GROUPS is the render order of the nav category headers.
export const SETTINGS_GROUPS: { id: SettingsGroup; labelKey: string; fallback: string }[] = [
  { id: 'personal', labelKey: 'settings.group.personal', fallback: 'Personal' },
  { id: 'modelsCost', labelKey: 'settings.group.modelsCost', fallback: 'Models & Cost' },
  { id: 'coding', labelKey: 'settings.group.coding', fallback: 'Coding' },
  { id: 'integrations', labelKey: 'settings.group.integrations', fallback: 'Integrations' },
  { id: 'workspace', labelKey: 'settings.group.workspace', fallback: 'Workspace' },
  { id: 'system', labelKey: 'settings.group.system', fallback: 'System' },
]

// SETTINGS_SECTIONS is the canonical nav model: id + i18n key + English fallback + group + icon.
// Exported so tests and the command palette can enumerate sections.
export const SETTINGS_SECTIONS: SettingsSection[] = [
  { id: 'general', labelKey: 'settings.general', fallback: 'General', group: 'personal', icon: IconSettings },
  { id: 'appearance', labelKey: 'settings.appearance', fallback: 'Appearance', group: 'personal', icon: IconPalette },
  { id: 'keybindings', labelKey: 'settings.keybindings', fallback: 'Keybindings', group: 'personal', icon: IconCommand },
  { id: 'language', labelKey: 'settings.language', fallback: 'Language', group: 'personal', icon: IconLanguages },
  { id: 'models', labelKey: 'settings.models', fallback: 'Models & Routing', group: 'modelsCost', icon: IconModel },
  { id: 'budget', labelKey: 'settings.budget', fallback: 'Budget & Cache', group: 'modelsCost', icon: IconCoins },
  { id: 'providers', labelKey: 'settings.providers', fallback: 'Providers & Keys', group: 'modelsCost', icon: IconKey },
  { id: 'permissions', labelKey: 'settings.permissions', fallback: 'Permissions & Autonomy', group: 'coding', icon: IconShield },
  { id: 'sandbox', labelKey: 'settings.sandbox', fallback: 'Sandbox', group: 'coding', icon: IconSandbox },
  { id: 'editor', labelKey: 'settings.editor', fallback: 'Editor & Diff', group: 'coding', icon: IconEditor },
  { id: 'context', labelKey: 'settings.context', fallback: 'Context & Memory', group: 'coding', icon: IconDatabase },
  { id: 'duet', labelKey: 'settings.duet', fallback: 'Duet', group: 'coding', icon: IconDuet },
  { id: 'extensions', labelKey: 'settings.extensions', fallback: 'Extensions', group: 'integrations', icon: IconExtensions },
  { id: 'network', labelKey: 'settings.network', fallback: 'Network / Proxy', group: 'integrations', icon: IconNetwork },
  { id: 'sessions', labelKey: 'settings.sessions', fallback: 'Sessions & Storage', group: 'workspace', icon: IconSessions },
  { id: 'doctor', labelKey: 'settings.doctor', fallback: 'Doctor', group: 'system', icon: IconActivity },
  { id: 'updates', labelKey: 'settings.updates', fallback: 'Updates', group: 'system', icon: IconRefresh },
  { id: 'about', labelKey: 'settings.about', fallback: 'About', group: 'system', icon: IconInfo },
]

export interface SettingsWindowProps {
  open: boolean
  onClose: () => void
}

export function SettingsWindow({ open, onClose }: SettingsWindowProps) {
  const [active, setActive] = useState('general')
  if (!open) return null

  const section = SETTINGS_SECTIONS.find((s) => s.id === active)

  return (
    <div className={styles.overlay} role="dialog" aria-modal="true" aria-label={t('settings.title', 'Settings')}>
      <div className={styles.window}>
        <nav className={styles.rail} aria-label={t('settings.title', 'Settings')}>
          {SETTINGS_SECTIONS.map((s) => (
            <button
              key={s.id}
              className={active === s.id ? `${styles.railBtn} ${styles.active}` : styles.railBtn}
              aria-current={active === s.id ? 'page' : undefined}
              onClick={() => setActive(s.id)}
            >
              {t(s.labelKey, s.fallback)}
            </button>
          ))}
        </nav>
        <section className={styles.panel}>{renderSection(active, section)}</section>
        <button className={styles.close} aria-label={t('settings.close', 'Close')} onClick={onClose}>
          <X size={16} />
        </button>
      </div>
    </div>
  )
}

function renderSection(active: string, section: SettingsSection | undefined) {
  switch (active) {
    case 'general':
    case 'language':
      return <GeneralSection />
    case 'appearance':
      return <AppearanceSection />
    case 'keybindings':
      return <KeybindingsSection />
    case 'providers':
      return <ProvidersSection />
    case 'models':
      return <ModelsSection />
    case 'budget':
      return <BudgetSection />
    case 'duet':
      return <DuetSection />
    case 'permissions':
      return <PermissionsSection />
    case 'sandbox':
      return <SandboxSection />
    case 'editor':
      return <EditorSection />
    case 'context':
      return <ContextSection />
    case 'extensions':
      return <ExtensionsSection />
    case 'network':
      return <NetworkSection />
    case 'sessions':
      return <SessionsSection />
    case 'doctor':
      return <DoctorView />
    case 'updates':
    case 'about':
      return <UpdatesSection />
    default:
      return (
        <div className={styles.placeholder}>
          <h2>{t(section?.labelKey ?? '', section?.fallback ?? '')}</h2>
          <p>{t('settings.ownedByWave', 'This section is provided by its owning wave.')}</p>
        </div>
      )
  }
}
