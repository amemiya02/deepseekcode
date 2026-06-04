// Adapted from deepseek-reasonix (MIT) — SettingsPanel.tsx (nav-rail + active tab + section switch).
import { useState } from 'react'
import { X } from 'lucide-react'
import { t } from '../../lib/i18n'
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

export interface SettingsSection {
  id: string
  labelKey: string
  fallback: string
}

// SETTINGS_SECTIONS is the canonical nav model: id + i18n key + English fallback.
// Exported so tests and the command palette can enumerate sections.
export const SETTINGS_SECTIONS: SettingsSection[] = [
  { id: 'general', labelKey: 'settings.general', fallback: 'General' },
  { id: 'appearance', labelKey: 'settings.appearance', fallback: 'Appearance' },
  { id: 'keybindings', labelKey: 'settings.keybindings', fallback: 'Keybindings' },
  { id: 'providers', labelKey: 'settings.providers', fallback: 'Providers & Keys' },
  { id: 'models', labelKey: 'settings.models', fallback: 'Models & Routing' },
  { id: 'budget', labelKey: 'settings.budget', fallback: 'Budget & Cache' },
  { id: 'duet', labelKey: 'settings.duet', fallback: 'Duet' },
  { id: 'permissions', labelKey: 'settings.permissions', fallback: 'Permissions & Autonomy' },
  { id: 'sandbox', labelKey: 'settings.sandbox', fallback: 'Sandbox' },
  { id: 'editor', labelKey: 'settings.editor', fallback: 'Editor & Diff' },
  { id: 'context', labelKey: 'settings.context', fallback: 'Context & Memory' },
  { id: 'extensions', labelKey: 'settings.extensions', fallback: 'Extensions' },
  { id: 'network', labelKey: 'settings.network', fallback: 'Network / Proxy' },
  { id: 'sessions', labelKey: 'settings.sessions', fallback: 'Sessions & Storage' },
  { id: 'language', labelKey: 'settings.language', fallback: 'Language' },
  { id: 'doctor', labelKey: 'settings.doctor', fallback: 'Doctor' },
  { id: 'updates', labelKey: 'settings.updates', fallback: 'Updates' },
  { id: 'about', labelKey: 'settings.about', fallback: 'About' },
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
