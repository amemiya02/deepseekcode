// Live message timeline: renders streamed transcript items as distinct blocks.
// Each block type (reasoning, tool, command, file-change) gets its own visual
// treatment and data-testid for E2E / unit targeting.
import {
  Check, FileDiff, Loader2, SquareTerminal, Terminal, Wrench, X,
} from 'lucide-react'

// ── Item types ────────────────────────────────────────────────────────────────

export interface ReasoningItem {
  type: 'reasoning'
  text: string
  streaming?: boolean
}

export interface ToolTimelineItem {
  type: 'tool'
  name: string
  args: Record<string, unknown>
  readOnly: boolean
  status: 'running' | 'ok' | 'error'
}

export interface CommandItem {
  type: 'command'
  command: string
  output?: string
}

export interface FileChangeItem {
  type: 'file-change'
  path: string
  added: number
  removed: number
}

export type TimelineItem = ReasoningItem | ToolTimelineItem | CommandItem | FileChangeItem

// ── Helpers ───────────────────────────────────────────────────────────────────

function subjectOf(args: Record<string, unknown>): string {
  const v = args.path ?? args.file ?? args.pattern ?? args.command
  return typeof v === 'string' ? v : ''
}

function StatusGlyph({ status }: { status: ToolTimelineItem['status'] }) {
  if (status === 'running') return <Loader2 className="ico spin" size={13} data-testid="timeline-tool-spinner" />
  if (status === 'error') return <X className="ico ico--err" size={13} />
  return <Check className="ico ico--ok" size={13} />
}

// ── Block renderers ───────────────────────────────────────────────────────────

function ReasoningBlock({ item }: { item: ReasoningItem }) {
  return (
    <div
      className={`tl-block tl-block--reasoning ${item.streaming ? 'ds-streaming-text' : ''}`}
      data-testid="block-reasoning"
    >
      <span className="tl-block__text">{item.text}</span>
      {item.streaming && <span className="cursor" />}
    </div>
  )
}

function ToolBlock({ item }: { item: ToolTimelineItem }) {
  const subject = subjectOf(item.args)
  return (
    <div className={`tl-block tl-block--tool tl-block--tool-${item.status}`} data-testid="block-tool">
      <span className="tl-block__icon" aria-hidden="true">
        {item.name === 'bash' ? <Terminal size={14} /> : <Wrench size={14} />}
      </span>
      <span className="tl-block__name">{item.name}</span>
      {subject && <span className="tl-block__subject">{subject}</span>}
      <span className="tl-block__status"><StatusGlyph status={item.status} /></span>
    </div>
  )
}

function CommandBlock({ item }: { item: CommandItem }) {
  return (
    <div className="tl-block tl-block--command" data-testid="block-command">
      <div className="tl-block__cmd-head">
        <SquareTerminal size={14} className="tl-block__icon" aria-hidden="true" />
        <span className="tl-block__cmd">{item.command}</span>
      </div>
      {item.output && <pre className="tl-block__output">{item.output}</pre>}
    </div>
  )
}

function FileChangeBlock({ item }: { item: FileChangeItem }) {
  return (
    <div className="tl-block tl-block--file-change" data-testid="block-file-change">
      <FileDiff size={14} className="tl-block__icon" aria-hidden="true" />
      <span className="tl-block__path">{item.path}</span>
      <span className="tl-block__counts">
        {item.added > 0 && <span className="tl-block__added">+{item.added}</span>}
        {item.removed > 0 && <span className="tl-block__removed">-{item.removed}</span>}
      </span>
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export function MessageTimeline({ items }: { items: TimelineItem[] }) {
  return (
    <div className="message-timeline" data-testid="message-timeline">
      {items.map((item, i) => {
        switch (item.type) {
          case 'reasoning':     return <ReasoningBlock key={i} item={item} />
          case 'tool':          return <ToolBlock key={i} item={item} />
          case 'command':       return <CommandBlock key={i} item={item} />
          case 'file-change':   return <FileChangeBlock key={i} item={item} />
        }
      })}
    </div>
  )
}
