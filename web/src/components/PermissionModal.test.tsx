import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { PermissionModal } from './PermissionModal'
import type { PermissionDecision, PermissionRequest } from '../lib/api'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

const req: PermissionRequest = {
  id: 'perm-1',
  tool: 'bash',
  args: { command: 'rm -rf build' },
  options: [],
}

describe('PermissionModal', () => {
  it('renders nothing when not open', () => {
    wrap(<PermissionModal open={false} request={req} />)
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('renders the card when open', () => {
    wrap(<PermissionModal open request={req} />)
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('bash')).toBeInTheDocument()
  })

  it('forwards the card decision via onDecide', async () => {
    const decisions: PermissionDecision[] = []
    const user = userEvent.setup()
    wrap(<PermissionModal open request={req} onDecide={(d) => decisions.push(d)} />)
    await user.click(screen.getByTestId('perm-session'))
    expect(decisions).toEqual(['session'])
  })

  it('Escape key fires onDecide("deny")', async () => {
    const decisions: PermissionDecision[] = []
    const user = userEvent.setup()
    wrap(<PermissionModal open request={req} onDecide={(d) => decisions.push(d)} />)
    await user.keyboard('{Escape}')
    expect(decisions).toEqual(['deny'])
  })

  it('backdrop click fires onDecide("deny")', async () => {
    const decisions: PermissionDecision[] = []
    const user = userEvent.setup()
    wrap(<PermissionModal open request={req} onDecide={(d) => decisions.push(d)} />)
    await user.click(screen.getByTestId('perm-backdrop'))
    expect(decisions).toEqual(['deny'])
  })

  it('number keys 1-4 map to once/session/always/deny', async () => {
    const decisions: PermissionDecision[] = []
    const user = userEvent.setup()
    wrap(<PermissionModal open request={req} onDecide={(d) => decisions.push(d)} />)
    await user.keyboard('1')
    await user.keyboard('2')
    await user.keyboard('3')
    await user.keyboard('4')
    expect(decisions).toEqual(['once', 'session', 'always', 'deny'])
  })
})
