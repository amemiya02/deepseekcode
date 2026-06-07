// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/Transcript.tsx
// Maps the reducer's TranscriptItem[] to the matching component, in order.
import type { TranscriptItem } from '../lib/transcript'
import type { RewindScope } from '../lib/checkpoint'
import { UserMessage } from './UserMessage'
import { AssistantMessage } from './AssistantMessage'
import { ThinkingBlock } from './ThinkingBlock'
import { ToolCard } from './ToolCard'
import { RoutingActivity } from './RoutingActivity'
import { DuetBadge } from './DuetBadge'
import { Logo } from './Logo'
import { MessageTimeline } from './MessageTimeline'
import type { TimelineItem } from './MessageTimeline'
import { t } from '../lib/i18n'

export interface TranscriptRewindHandlers {
  onRewind?: (keepMessages: number, scope: RewindScope) => void
  onFork?: () => void
  onSummarize?: (mode: 'from' | 'upto', index: number) => void
}

export function Transcript({ items, rewindHandlers }: { items: TranscriptItem[]; rewindHandlers?: TranscriptRewindHandlers }) {
  return (
    <div className="transcript" data-testid="transcript">
      {items.length === 0 && (
        <div className="empty-convo" data-testid="empty-convo">
          <div className="empty-convo__mark">
            <Logo size={32} />
          </div>
          <h2 className="empty-convo__title">{t('empty.title', 'Ready when you are')}</h2>
          <p className="empty-convo__sub">
            {t(
              'empty.sub',
              'Ask a question, plan a change, or build something — cache, cost, and routing stay in view the whole way.',
            )}
          </p>
        </div>
      )}
      <div className="transcript__lane">
        {items.map((item, i) => {
          switch (item.type) {
            case 'user':
              return (
                <UserMessage
                  key={i}
                  text={item.text}
                  pills={item.pills}
                  messageIndex={i}
                  onRewind={rewindHandlers?.onRewind}
                  onFork={rewindHandlers?.onFork}
                  onSummarize={rewindHandlers?.onSummarize}
                />
              )
            case 'assistant':
              return <AssistantMessage key={i} text={item.text} streaming={item.streaming} />
            case 'thinking':
              return <ThinkingBlock key={i} text={item.text} startedAt={item.startedAt} endedAt={item.endedAt} />
            case 'tool':
              return <ToolCard key={item.id} item={item} />
            case 'routing':
              return <RoutingActivity key={i} from={item.from} to={item.to} reason={item.reason} />
            case 'duet':
              return <DuetBadge key={i} decision={item.decision} reason={item.reason} />
          }
        })}
      </div>
      {(() => {
        const timelineItems: TimelineItem[] = items
          .filter((item): item is Extract<TranscriptItem, { type: 'tool' }> => item.type === 'tool')
          .map((item) => ({
            type: 'tool' as const,
            name: item.name,
            args: item.args,
            readOnly: item.readOnly,
            status: item.status,
          }))
        return timelineItems.length > 0 ? <MessageTimeline items={timelineItems} /> : null
      })()}
    </div>
  )
}
