// Adapted from deepseek-reasonix (MIT) — CapabilitiesPanel.tsx + MemoryPanel.tsx (tabbed reload-on-switch panels).
import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { IconExtensions, IconServer, IconActivity, IconDatabase } from '../../lib/icons'
import { StateView } from '../StateViews'
import parentStyles from './sections.module.css'
import styles from './ExtensionsSection.module.css'

interface ExtItem {
  id: string
  name: string
  enabled: boolean
}

const TABS = [
  { id: 'skills', path: '/v1/skills', labelKey: 'settings.skills', fallback: 'Skills', icon: IconExtensions },
  { id: 'hooks', path: '/v1/hooks', labelKey: 'settings.hooks', fallback: 'Hooks', icon: IconActivity },
  { id: 'memory', path: '/v1/memory', labelKey: 'settings.memory', fallback: 'Memory', icon: IconDatabase },
] as const

type TabId = (typeof TABS)[number]['id']

export function ExtensionsSection() {
  const [active, setActive] = useState<TabId>('skills')
  const [items, setItems] = useState<ExtItem[]>([])
  const [loading, setLoading] = useState(true)

  // Read-only enumeration of the real subsystems via /v1/{skills,hooks,memory}
  // (internal/gateway/extensions.go), each returning 200 {"items":[{id,name,enabled}]}.
  // These endpoints never 404 and return 200 {"items":[]} when nothing is
  // configured — so an unreachable/empty subsystem falls through to the honest
  // "Nothing configured yet." empty state instead of an error banner.
  async function loadTab(id: TabId) {
    const tab = TABS.find((x) => x.id === id)!
    setLoading(true)
    setItems([])
    try {
      const res = await fetch(tab.path, { headers: { Accept: 'application/json' } })
      if (!res.ok) {
        setItems([])
        return
      }
      const body = await res.json()
      setItems((body.items ?? []) as ExtItem[])
    } catch {
      // Network/parse failures degrade to the empty state — never an error.
      setItems([])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadTab(active)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active])

  const activeTab = TABS.find((t) => t.id === active)!

  return (
    <div>
      <div className={parentStyles.header}>
        <h2 className={parentStyles.h2}>{t('settings.extensions', 'Extensions')}</h2>
        <p className={parentStyles.sub}>{t('settings.extensionsSub', 'Skills, hooks, and memory providers.')}</p>
      </div>

      <div className={styles.tabs} role="tablist">
        {TABS.map((tab) => (
          <button
            key={tab.id}
            role="tab"
            aria-selected={active === tab.id}
            className={active === tab.id ? `${styles.tab} ${styles.tabActive}` : styles.tab}
            onClick={() => setActive(tab.id)}
          >
            {t(tab.labelKey, tab.fallback)}
          </button>
        ))}
      </div>

      {loading ? (
        <StateView kind="loading" message={t('settings.loading', 'Loading…')} />
      ) : items.length === 0 ? (
        <StateView kind="empty" title={t('settings.extensionsEmpty', 'Nothing configured yet.')} />
      ) : (
        <ul className={styles.list}>
          {items.map((it) => {
            const TabIcon = activeTab.icon
            return (
              <li key={it.id} className={styles.item}>
                <div className={styles.itemIcon}>
                  <TabIcon size={16} />
                </div>
                <div className={styles.itemBody}>
                  <div className={styles.itemName}>{it.name}</div>
                  <div className={styles.itemId}>{it.id}</div>
                </div>
                <span className={`${styles.status} ${it.enabled ? styles.statusOn : styles.statusOff}`}>
                  <span className={`${styles.dot} ${it.enabled ? styles.dotOn : styles.dotOff}`} />
                  {it.enabled ? t('settings.enabled', 'enabled') : t('settings.disabled', 'disabled')}
                </span>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
