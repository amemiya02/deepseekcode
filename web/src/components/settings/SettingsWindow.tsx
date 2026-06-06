// Adapted from deepseek-reasonix (MIT) — SettingsPanel.tsx (nav-rail + active tab + section switch).
import { useMemo, useState } from 'react'
import { t } from '../../lib/i18n'
import {
  IconSettings, IconPalette, IconCommand, IconLanguages, IconModel, IconCoins, IconKey,
  IconShield, IconSandbox, IconEditor, IconDatabase, IconDuet, IconExtensions, IconNetwork,
  IconSessions, IconActivity, IconRefresh, IconInfo, type Icon,
  IconChevronLeft, IconSearch,
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

export interface SettingsViewProps {
  onClose: () => void
}

export function SettingsView({ onClose }: SettingsViewProps) {
  const [active, setActive] = useState('general')
  const [query, setQuery] = useState('')

  const q = query.trim().toLowerCase()
  const matches = useMemo(
    () => SETTINGS_SECTIONS.filter((s) => (q ? t(s.labelKey, s.fallback).toLowerCase().includes(q) : true)),
    [q],
  )
  const section = SETTINGS_SECTIONS.find((s) => s.id === active)

  return (
    <div className={styles.view}>
      <nav className={styles.nav} aria-label={t('settings.title', 'Settings')}>
        <button className={styles.back} data-testid="settings-back" onClick={onClose}>
          <IconChevronLeft size={16} />
          <span>{t('settings.back', 'Back to app')}</span>
        </button>
        <label className={styles.search}>
          <IconSearch size={14} aria-hidden="true" />
          <input
            data-testid="settings-search"
            type="search"
            placeholder={t('settings.search', 'Search settings…')}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Escape') setQuery('') }}
          />
        </label>

        {matches.length === 0 ? (
          <p className={styles.searchEmpty}>{t('settings.searchEmpty', 'No matching settings.')}</p>
        ) : (
          SETTINGS_GROUPS.map((group) => {
            const items = matches.filter((s) => s.group === group.id)
            if (items.length === 0) return null
            return (
              <div className={styles.group} key={group.id}>
                <div className={styles.groupLabel}>{t(group.labelKey, group.fallback)}</div>
                {items.map((s) => {
                  const Icon = s.icon
                  return (
                    <button
                      key={s.id}
                      data-testid="settings-nav-item"
                      className={active === s.id ? `${styles.navItem} ${styles.active}` : styles.navItem}
                      aria-current={active === s.id ? 'page' : undefined}
                      onClick={() => setActive(s.id)}
                    >
                      <Icon size={15} aria-hidden="true" />
                      <span>{t(s.labelKey, s.fallback)}</span>
                    </button>
                  )
                })}
              </div>
            )
          })
        )}
      </nav>
      <section className={styles.panel}>{renderSection(active, section)}</section>
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
