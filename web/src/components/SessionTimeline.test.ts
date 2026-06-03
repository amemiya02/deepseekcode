import { render, screen, within } from '@testing-library/svelte'
import { describe, it, expect } from 'vitest'
import SessionTimeline from './SessionTimeline.svelte'

describe('SessionTimeline', () => {
  it('renders epoch list from prop', () => {
    render(SessionTimeline, {
      props: {
        sessionId: 'sess-1',
        epochs: [
          { id: 'e1', turns: 4, compacted: false },
          { id: 'e2', turns: 2, compacted: true },
        ],
      },
    })

    // Exact id matchers to avoid false passes on substrings
    expect(screen.getByText('e1')).toBeTruthy()
    expect(screen.getByText('e2')).toBeTruthy()

    // Turns text and pluralization
    expect(screen.getByText('4 turns')).toBeTruthy()
    expect(screen.getByText('2 turns')).toBeTruthy()

    // Badge must appear in e2's list item but NOT in e1's list item
    const items = screen.getAllByRole('listitem')
    const e1Item = items.find((li) => li.textContent?.includes('e1'))!
    const e2Item = items.find((li) => li.textContent?.includes('e2'))!
    expect(within(e2Item).queryByTestId('badge-compacted')).not.toBeNull()
    expect(within(e1Item).queryByTestId('badge-compacted')).toBeNull()
  })

  it('uses singular "turn" when epoch has exactly 1 turn', () => {
    render(SessionTimeline, {
      props: {
        sessionId: 'sess-2',
        epochs: [{ id: 'e3', turns: 1, compacted: false }],
      },
    })
    expect(screen.getByText('1 turn')).toBeTruthy()
  })

  it('shows empty state when no epochs', () => {
    render(SessionTimeline, { props: { sessionId: '', epochs: [] } })
    expect(screen.getByTestId('timeline-empty')).toBeTruthy()
  })
})
