import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import React from 'react'
import { LocaleProvider } from '../lib/i18n'
import { ApprovalGate } from './ApprovalGate'
import type { PermissionRequest } from '../lib/api'

const REQ: PermissionRequest = { id: 'p1', tool: 'edit_file', args: { path: 'a.ts', old_string: 'a', new_string: 'b' }, options: [] }
const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('ApprovalGate', () => {
  it('renders the proposed diff as an obsidian island showing the path', () => {
    const { container } = wrap(<ApprovalGate request={REQ} onDecide={vi.fn()} />)
    expect(container.querySelector('.island')).not.toBeNull()
    expect(screen.getByText('a.ts')).toBeInTheDocument()
  })
  it('accept fires once; deny fires deny', async () => {
    const calls: string[] = []
    wrap(<ApprovalGate request={REQ} onDecide={(d) => calls.push(d)} />)
    await userEvent.click(screen.getByTestId('approve-once'))
    await userEvent.click(screen.getByTestId('approve-deny'))
    expect(calls).toEqual(['once', 'deny'])
  })
  it('the more-menu exposes session and always', async () => {
    const calls: string[] = []
    wrap(<ApprovalGate request={REQ} onDecide={(d) => calls.push(d)} />)
    await userEvent.click(screen.getByTestId('approve-more'))
    await userEvent.click(screen.getByTestId('approve-session'))
    await userEvent.click(screen.getByTestId('approve-more'))
    await userEvent.click(screen.getByTestId('approve-always'))
    expect(calls).toEqual(['session', 'always'])
  })
  it('Enter accepts once and Backspace denies', async () => {
    const calls: string[] = []
    wrap(<ApprovalGate request={REQ} onDecide={(d) => calls.push(d)} />)
    screen.getByTestId('approval-gate').focus()
    await userEvent.keyboard('{Enter}')
    await userEvent.keyboard('{Backspace}')
    expect(calls).toEqual(['once', 'deny'])
  })
})
