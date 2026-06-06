// Adapted from deepseek-reasonix (MIT) — CapabilitiesPanel.tsx + MemoryPanel.tsx (tabbed reload-on-switch panels).
import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { StateView } from '../StateViews'
import styles from './ExtensionsSection.module.css'

interface ExtItem {
  id: string
  name: string
  enabled: boolean
}

const TABS = [
  { id: 'skills', path: '/v1/skills', labelKey: 'settings.skills', fallback: 'Skills' },
  { id: 'hooks', path: '/v1/hooks', labelKey: 'settings.hooks', fallback: 'Hooks' },
  { id: 'memory', path: '/v1/memory', labelKey: 'settings.memory', fallback: 'Memory' },
] as const

type TabId = (typeof TABS)[number]['id']

export function ExtensionsSection() {
  const [active, setActive] = useState<TabId>('skills')
  const [items, setItems] = useState<ExtItem[]>([])
  const [loading, setLoading] = useState(true)

  // Read-only enumeration of the real subsystems via /v1/{mcp,skills,hooks,memory}
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
        // Defensive: a non-200 (e.g. a stale gateway without the route) is
        // rendered as the empty state, not a low-level error banner.
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

  return (
    <div>
      <h2 className={styles.h2}>{t('settings.extensions', 'Extensions')}</h2>
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
          {items.map((it) => (
            <li key={it.id}>
              <span className={styles.name}>{it.name}</span>
              <span className={it.enabled ? `${styles.badge} ${styles.on}` : styles.badge}>
                {it.enabled ? t('settings.enabled', 'enabled') : t('settings.disabled', 'disabled')}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
