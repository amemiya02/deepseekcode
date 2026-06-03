import { render, screen } from '@testing-library/svelte'
import { describe, it, expect } from 'vitest'
import ConversationView from './ConversationView.svelte'

describe('ConversationView', () => {
  it('renders submitted prompt text', async () => {
    render(ConversationView, {
      props: { messages: [{ role: 'user', text: 'fix the nil deref' }] },
    })
    expect(screen.getByText('fix the nil deref')).toBeTruthy()
  })

  it('renders assistant delta text', () => {
    render(ConversationView, {
      props: {
        messages: [
          { role: 'user', text: 'hi' },
          { role: 'assistant', text: 'I found the bug.' },
        ],
      },
    })
    expect(screen.getByText('I found the bug.')).toBeTruthy()
  })

  it('renders tool call row', () => {
    render(ConversationView, {
      props: {
        messages: [{ role: 'tool', text: 'read_file({"path":"main.go"})' }],
      },
    })
    expect(screen.getByText(/read_file/)).toBeTruthy()
  })

  it('shows streaming indicator when isStreaming is true', () => {
    render(ConversationView, {
      props: { messages: [], isStreaming: true },
    })
    expect(screen.getByTestId('streaming-indicator')).toBeTruthy()
  })
})
