import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { PlanTodoPanel } from './PlanTodoPanel'
import type { PlanItem } from '../lib/api'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

const items: PlanItem[] = [
  { text: 'write test', status: 'done' },
  { text: 'implement', status: 'in_progress' },
  { text: 'commit', status: 'pending' },
]

describe('PlanTodoPanel', () => {
  it('renders each item and a progress count', () => {
    wrap(<PlanTodoPanel items={items} />)
    expect(screen.getByText('write test')).toBeInTheDocument()
    expect(screen.getByText('implement')).toBeInTheDocument()
    expect(screen.getByTestId('plan-progress').textContent).toContain('1/3')
  })

  it('marks each item with a per-status data attribute', () => {
    const { container } = wrap(<PlanTodoPanel items={items} />)
    const statuses = Array.from(container.querySelectorAll('[data-status]')).map((el) =>
      el.getAttribute('data-status'),
    )
    expect(statuses).toEqual(['done', 'in_progress', 'pending'])
  })

  it('renders nothing when items is empty', () => {
    wrap(<PlanTodoPanel items={[]} />)
    expect(screen.queryByTestId('plan-panel')).toBeNull()
  })

  it('renders nothing when all items are done (auto-clear)', () => {
    wrap(<PlanTodoPanel items={[{ text: 'x', status: 'done' }]} />)
    expect(screen.queryByTestId('plan-panel')).toBeNull()
  })

  it('dismiss button fires onDismiss', async () => {
    let dismissed = false
    const user = userEvent.setup()
    wrap(<PlanTodoPanel items={items} onDismiss={() => { dismissed = true }} />)
    await user.click(screen.getByTestId('plan-dismiss'))
    expect(dismissed).toBe(true)
  })

  it('collapse toggle hides the list but keeps the panel', async () => {
    const user = userEvent.setup()
    wrap(<PlanTodoPanel items={items} />)
    expect(screen.getByText('commit')).toBeInTheDocument()
    await user.click(screen.getByTestId('plan-toggle'))
    expect(screen.queryByText('commit')).toBeNull()
    expect(screen.getByTestId('plan-panel')).toBeInTheDocument()
  })
})
