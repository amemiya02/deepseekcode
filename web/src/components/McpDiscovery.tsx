import { useState } from 'react'
import styles from './McpDiscovery.module.css'

export interface McpToolResult {
  server: string
  tool: string
  desc: string
}

interface McpDiscoveryProps {
  results: McpToolResult[]
  onSearch: (query: string) => void
  onCall: (server: string, tool: string) => void
}

export function McpDiscovery({ results, onSearch, onCall }: McpDiscoveryProps) {
  const [query, setQuery] = useState('')

  function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const value = e.target.value
    setQuery(value)
    onSearch(value)
  }

  return (
    <div className={styles.container}>
      <div className={styles.searchBox}>
        <input
          type="text"
          placeholder="Search MCP tools..."
          value={query}
          onChange={handleChange}
          aria-label="Search MCP tools"
        />
      </div>
      {results.length > 0 && (
        <ul className={styles.list}>
          {results.map((r) => (
            <li key={`${r.server}/${r.tool}`} className={styles.row}>
              <div className={styles.rowMain}>
                <div className={styles.rowHeader}>
                  <span className={styles.toolName}>{r.tool}</span>
                  <span className={styles.serverBadge}>{r.server}</span>
                </div>
                <div className={styles.desc}>{r.desc}</div>
              </div>
              <button
                className={styles.callBtn}
                onClick={() => onCall(r.server, r.tool)}
                aria-label={`Call ${r.tool}`}
              >
                Call
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
