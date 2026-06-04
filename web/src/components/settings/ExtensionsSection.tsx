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
  { id: 'mcp', path: '/v1/mcp', labelKey: 'settings.mcp', fallback: 'MCP' },
  { id: 'skills', path: '/v1/skills', labelKey: 'settings.skills', fallback: 'Skills' },
  { id: 'hooks', path: '/v1/hooks', labelKey: 'settings.hooks', fallback: 'Hooks' },
  { id: 'memory', path: '/v1/memory', labelKey: 'settings.memory', fallback: 'Memory' },
] as const

type TabId = (typeof TABS)[number]['id']

export function ExtensionsSection() {
  const [active, setActive] = useState<TabId>('mcp')
  const [items, setItems] = useState<ExtItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  async function loadTab(id: TabId) {
    const tab = TABS.find((x) => x.id === id)!
    setLoading(true)
    setError('')
    setItems([])
    try {
      const res = await fetch(tab.path, { headers: { Accept: 'application/json' } })
      if (!res.ok) throw new Error(`gateway error ${res.status}`)
      const body = await res.json()
      setItems((body.items ?? []) as ExtItem[])
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
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
      ) : error ? (
        <StateView kind="error" message={error} onRetry={() => void loadTab(active)} />
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
