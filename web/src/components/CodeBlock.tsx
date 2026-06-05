import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { CodeIsland } from './CodeIsland'

function langOf(className?: string): string {
  const m = /language-([\w-]+)/.exec(className ?? '')
  return m ? m[1] : 'code'
}

function rawText(children: React.ReactNode): string {
  if (typeof children === 'string') return children
  if (Array.isArray(children)) return children.map(rawText).join('')
  if (children && typeof children === 'object' && 'props' in children) {
    return rawText((children as { props: { children?: React.ReactNode } }).props.children)
  }
  return ''
}

export function CodeBlock({ className, children }: { className?: string; children?: React.ReactNode }) {
  const [copied, setCopied] = useState(false)
  const code = rawText(children).replace(/\n$/, '')
  const lang = langOf(className)
  async function copy() {
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      setTimeout(() => setCopied(false), 1200)
    } catch { /* no-op: clipboard may be unavailable */ }
  }
  return (
    <CodeIsland
      label={<span className="codecard__lang" data-testid="codecard-lang">{lang}</span>}
      actions={
        <button className="island__copy" data-testid="codecard-copy" onClick={copy} type="button">
          {copied ? <Check size={13} /> : <Copy size={13} />}
          {copied ? 'Copied' : 'Copy'}
        </button>
      }
    >
      <pre className="island__pre"><code className={`hljs ${className ?? ''}`}>{children}</code></pre>
    </CodeIsland>
  )
}
