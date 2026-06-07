import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { MessageTimeline } from './MessageTimeline'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('MessageTimeline', () => {
  it('renders reasoning, tool, command, and file-change as distinct blocks', () => {
    wrap(
      <MessageTimeline
        items={[
          { type: 'reasoning', text: 'thinking' },
          { type: 'tool', name: 'read_file', args: { path: 'a.ts' }, readOnly: true, status: 'ok' },
          { type: 'command', command: 'npm test', output: 'ok' },
          { type: 'file-change', path: 'a.ts', added: 3, removed: 1 },
        ]}
      />,
    )
    expect(screen.getByTestId('block-reasoning')).toBeInTheDocument()
    expect(screen.getByTestId('block-tool')).toBeInTheDocument()
    expect(screen.getByTestId('block-command')).toBeInTheDocument()
    expect(screen.getByTestId('block-file-change')).toBeInTheDocument()
  })

  it('renders reasoning text inline', () => {
    wrap(<MessageTimeline items={[{ type: 'reasoning', text: 'analysing the codebase' }]} />)
    expect(screen.getByText('analysing the codebase')).toBeInTheDocument()
  })

  it('applies ds-streaming-text to a live reasoning block', () => {
    wrap(<MessageTimeline items={[{ type: 'reasoning', text: 'still thinking', streaming: true }]} />)
    const block = screen.getByTestId('block-reasoning')
    expect(block.className).toContain('ds-streaming-text')
  })

  it('renders tool name and subject from args', () => {
    wrap(
      <MessageTimeline
        items={[{ type: 'tool', name: 'read_file', args: { path: 'main.ts' }, readOnly: true, status: 'ok' }]}
      />,
    )
    expect(screen.getByText('read_file')).toBeInTheDocument()
    expect(screen.getByText('main.ts')).toBeInTheDocument()
  })

  it('renders a running tool with a spinner glyph', () => {
    wrap(
      <MessageTimeline
        items={[{ type: 'tool', name: 'bash', args: { command: 'ls' }, readOnly: false, status: 'running' }]}
      />,
    )
    expect(screen.getByTestId('timeline-tool-spinner')).toBeInTheDocument()
  })

  it('renders command output in a mono block', () => {
    wrap(<MessageTimeline items={[{ type: 'command', command: 'npm test', output: 'all 42 passed' }]} />)
    expect(screen.getByText('npm test')).toBeInTheDocument()
    expect(screen.getByText('all 42 passed')).toBeInTheDocument()
  })

  it('renders file-change with path and line counts', () => {
    wrap(<MessageTimeline items={[{ type: 'file-change', path: 'src/app.ts', added: 10, removed: 3 }]} />)
    expect(screen.getByText('src/app.ts')).toBeInTheDocument()
    expect(screen.getByText('+10')).toBeInTheDocument()
    expect(screen.getByText('-3')).toBeInTheDocument()
  })

  it('renders an empty items array without error', () => {
    wrap(<MessageTimeline items={[]} />)
    expect(screen.getByTestId('message-timeline')).toBeInTheDocument()
  })
})
