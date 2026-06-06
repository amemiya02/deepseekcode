import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { Transcript } from './Transcript'
import type { TranscriptItem } from '../lib/transcript'

const items: TranscriptItem[] = [
  { type: 'user', text: 'do it' },
  { type: 'thinking', text: 'reasoning' },
  { type: 'tool', id: 't1', name: 'read_file', args: { path: 'a' }, readOnly: true, status: 'ok', result: 'r' },
  { type: 'routing', from: 'flash', to: 'pro', reason: 'hard' },
  { type: 'assistant', text: 'done', streaming: false },
]

describe('Transcript', () => {
  it('renders items in order with the right component per type', () => {
    render(<LocaleProvider><Transcript items={items} /></LocaleProvider>)
    expect(screen.getByText('do it')).toBeInTheDocument()
    expect(screen.getByTestId('thinking-toggle')).toBeInTheDocument()
    expect(screen.getByText('read_file')).toBeInTheDocument()
    expect(screen.getByText('pro')).toBeInTheDocument()
    expect(screen.getByText('done')).toBeInTheDocument()
  })

  it('renders an empty container for no items', () => {
    const { container } = render(<LocaleProvider><Transcript items={[]} /></LocaleProvider>)
    expect(container.querySelector('.transcript')).toBeInTheDocument()
  })
})
