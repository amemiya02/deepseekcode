import { useEffect, useState } from 'react'
import { IconPlus, IconServer, IconTrash, IconSettings, IconChevronLeft } from '../../lib/icons'
import { t } from '../../lib/i18n'
import { StateView } from '../StateViews'
import { BrandedSelect } from '../BrandedSelect'
import { fetchMcpServers, saveMcpServer, deleteMcpServer, toggleMcpServer, type McpServer } from '../../lib/system'
import styles from './McpSection.module.css'

type Editing = { name: string; transport: string; command: string; url: string; args: string } | null

const TRANSPORTS = [
  { value: 'stdio', label: 'stdio' },
  { value: 'sse', label: 'sse' },
]

export function McpSection() {
  const [items, setItems] = useState<McpServer[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<Editing>(null)
  const [error, setError] = useState('')

  async function load() {
    setLoading(true)
    try { setItems(await fetchMcpServers()) } catch (e) { setError(String(e)) } finally { setLoading(false) }
  }
  useEffect(() => { void load() }, [])

  async function onToggle(s: McpServer) { setItems(await toggleMcpServer(s.name, s.enabled)) }
  async function onDelete(name: string) { setItems(await deleteMcpServer(name)) }
  async function onSave() {
    if (!editing || !editing.name.trim()) return
    setItems(await saveMcpServer({
      name: editing.name.trim(),
      transport: editing.transport,
      command: editing.command.trim() || undefined,
      url: editing.url.trim() || undefined,
      args: editing.args.trim() ? editing.args.trim().split(/\s+/) : undefined,
    }))
    setEditing(null)
  }

  if (editing) {
    return (
      <div>
        <button className={styles.back} data-testid="mcp-back" onClick={() => setEditing(null)}>
          <IconChevronLeft size={15} /> {t('mcp.back', 'MCP servers')}
        </button>
        <div className={styles.field}>
          <label>{t('mcp.name', 'Name')}</label>
          <input data-testid="mcp-field-name" value={editing.name} onChange={(e) => setEditing({ ...editing, name: e.target.value })} />
        </div>
        <div className={styles.field}>
          <label>{t('mcp.transport', 'Transport')}</label>
          <BrandedSelect value={editing.transport} options={TRANSPORTS} ariaLabel={t('mcp.transport', 'Transport')}
            onChange={(v) => setEditing({ ...editing, transport: v })} />
        </div>
        {editing.transport === 'sse' ? (
          <div className={styles.field}>
            <label>{t('mcp.url', 'URL')}</label>
            <input data-testid="mcp-field-url" value={editing.url} onChange={(e) => setEditing({ ...editing, url: e.target.value })} />
          </div>
        ) : (
          <div className={styles.field}>
            <label>{t('mcp.command', 'Command')}</label>
            <input data-testid="mcp-field-command" value={editing.command} onChange={(e) => setEditing({ ...editing, command: e.target.value })} />
          </div>
        )}
        <div className={styles.field}>
          <label>{t('mcp.args', 'Args (space-separated)')}</label>
          <input data-testid="mcp-field-args" value={editing.args} onChange={(e) => setEditing({ ...editing, args: e.target.value })} />
        </div>
        <div className={styles.actions}>
          <button className={styles.cancel} onClick={() => setEditing(null)}>{t('mcp.cancel', 'Cancel')}</button>
          <button className={styles.save} data-testid="mcp-save" onClick={onSave}>{t('mcp.save', 'Save')}</button>
        </div>
      </div>
    )
  }

  return (
    <div>
      <div className={styles.header}>
        <div>
          <h2 className={styles.h2}>{t('mcp.title', 'MCP servers')}</h2>
          <p className={styles.sub}>{t('mcp.subtitle', 'Connect external tools and data sources.')}</p>
        </div>
        <button className={styles.add} data-testid="mcp-add"
          onClick={() => setEditing({ name: '', transport: 'stdio', command: '', url: '', args: '' })}>
          <IconPlus size={14} /> {t('mcp.add', 'Add server')}
        </button>
      </div>
      {error && <StateView kind="error" message={error} />}
      {loading ? (
        <StateView kind="loading" message={t('settings.loading', 'Loading…')} />
      ) : items.length === 0 ? (
        <StateView kind="empty" title={t('mcp.empty', 'No MCP servers configured.')} />
      ) : (
        <ul className={styles.list}>
          {items.map((s) => (
            <li key={s.id} className={s.enabled ? styles.row : `${styles.row} ${styles.rowOff}`}>
              <IconServer size={16} className={styles.rowIcon} aria-hidden="true" />
              <div className={styles.rowMain}>
                <div className={styles.rowName}>
                  {s.name}
                  {s.status === 'connected' && (
                    <span className={styles.ok}>{t('mcp.connected', 'connected')}{s.toolCount ? ` · ${t('mcp.tools', '{n} tools', { n: s.toolCount })}` : ''}</span>
                  )}
                </div>
                <div className={styles.rowMeta}>{s.transport} · {s.command}</div>
              </div>
              <button className={styles.iconBtn} aria-label={t('mcp.edit', 'Edit server')}
                onClick={() => setEditing({ name: s.name, transport: s.transport, command: s.command, url: '', args: '' })}>
                <IconSettings size={15} />
              </button>
              <button className={styles.iconBtn} aria-label={t('mcp.delete', 'Remove server')} onClick={() => onDelete(s.name)}>
                <IconTrash size={15} />
              </button>
              <button className={s.enabled ? `${styles.toggle} ${styles.toggleOn}` : styles.toggle}
                role="switch" aria-checked={s.enabled} data-testid={`mcp-toggle-${s.name}`}
                aria-label={t('mcp.enableToggle', 'Enable / disable')} onClick={() => onToggle(s)}>
                <span className={styles.knob} />
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
