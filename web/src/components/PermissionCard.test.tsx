import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { PermissionCard } from './PermissionCard'
import type { PermissionDecision, PermissionRequest } from '../lib/api'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

const req: PermissionRequest = {
  id: 'perm-1',
  tool: 'bash',
  args: { command: 'rm -rf build' },
  options: [],
}

describe('PermissionCard', () => {
  it('renders the tool name and args summary', () => {
    wrap(<PermissionCard request={req} />)
    expect(screen.getByText('bash')).toBeInTheDocument()
    expect(screen.getByText(/rm -rf build/)).toBeInTheDocument()
  })

  it('calls onDecide with the chosen decision for each button', async () => {
    const decisions: PermissionDecision[] = []
    const user = userEvent.setup()
    wrap(<PermissionCard request={req} onDecide={(d) => decisions.push(d)} />)
    await user.click(screen.getByTestId('perm-deny'))
    await user.click(screen.getByTestId('perm-once'))
    await user.click(screen.getByTestId('perm-session'))
    await user.click(screen.getByTestId('perm-always'))
    expect(decisions).toEqual(['deny', 'once', 'session', 'always'])
  })

  it('does not throw when onDecide is undefined', async () => {
    const user = userEvent.setup()
    wrap(<PermissionCard request={req} />)
    await user.click(screen.getByTestId('perm-once'))
    expect(screen.getByTestId('perm-once')).toBeInTheDocument()
  })
})
