// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/Transcript.tsx
// Maps the reducer's TranscriptItem[] to the matching component, in order.
import type { TranscriptItem } from '../lib/transcript'
import type { RewindScope } from '../lib/checkpoint'
import { UserMessage } from './UserMessage'
import { AssistantMessage } from './AssistantMessage'
import { ThinkingBlock } from './ThinkingBlock'
import { ToolCard } from './ToolCard'
import { RoutingActivity } from './RoutingActivity'

export interface TranscriptRewindHandlers {
  onRewind?: (keepMessages: number, scope: RewindScope) => void
  onFork?: () => void
  onSummarize?: (mode: 'from' | 'upto', index: number) => void
}

export function Transcript({ items, rewindHandlers }: { items: TranscriptItem[]; rewindHandlers?: TranscriptRewindHandlers }) {
  return (
    <div className="transcript" data-testid="transcript">
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
            return <ThinkingBlock key={i} text={item.text} />
          case 'tool':
            return <ToolCard key={item.id} item={item} />
          case 'routing':
            return <RoutingActivity key={i} from={item.from} to={item.to} reason={item.reason} />
        }
      })}
    </div>
  )
}
