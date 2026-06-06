// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/ToolCard.tsx
// Adaptive card: read-only "research" calls (read/grep/ls/glob/web_fetch) collapse
// to a quiet dim row; writers/bash/errors keep the full card expanded. Uses the
// readOnly flag (not a tool-name list) per spec §7.A.
import { useState } from 'react'
import {
  Check, ChevronRight, FilePen, FileText, FolderOpen, Globe, Loader2, ListTree,
  Search, SquareTerminal, Wrench, X, type LucideIcon,
} from 'lucide-react'
import { useT } from '../lib/i18n'
import type { ToolItem } from '../lib/transcript'

const ICONS: Record<string, LucideIcon> = {
  edit_file: FilePen, multi_edit: FilePen, write_file: FilePen,
  read_file: FileText, bash: SquareTerminal, ls: FolderOpen,
  glob: Search, grep: Search, web_fetch: Globe, task: ListTree,
}

function StatusGlyph({ status }: { status: ToolItem['status'] }) {
  if (status === 'running') return <Loader2 className="ico spin" size={13} data-testid="toolcard-spinner" />
  if (status === 'error') return <X className="ico ico--err" size={13} />
  return <Check className="ico ico--ok" size={13} />
}

function subjectOf(args: Record<string, unknown>): string {
  const v = args.path ?? args.file ?? args.pattern ?? args.command
  return typeof v === 'string' ? v : ''
}

export function ToolCard({ item }: { item: ToolItem }) {
  const t = useT()
  const Icon = ICONS[item.name] ?? Wrench
  // Writers (and anything non-readOnly, errored, or still running) start expanded;
  // a settled read-only call starts collapsed and quiet.
  const quiet = item.readOnly && item.status !== 'error' && item.status !== 'running'
  const [open, setOpen] = useState(!quiet)
  const subject = subjectOf(item.args)

  return (
    <div className={`tool tool--${item.status} ${quiet ? 'tool--quiet' : ''}`}>
      <button className="tool__row" data-testid="toolcard-header" onClick={() => setOpen((v) => !v)} aria-expanded={open}>
        <ChevronRight className={`tool__chevron ${open ? 'tool__chevron--open' : ''}`} size={13} />
        <Icon className="tool__icon" size={14} />
        <span className="tool__name">{item.name}</span>
        {subject && <span className="tool__subject">{subject}</span>}
        <span className="tool__meta"><StatusGlyph status={item.status} /></span>
      </button>
      {open && item.result && <pre className={`tool__body ${item.status === 'error' ? 'tool__body--err' : ''}`}>{item.result}</pre>}
      {open && item.truncated && <div className="tool__note">{t('tool.truncated', 'Output truncated')}</div>}
    </div>
  )
}
