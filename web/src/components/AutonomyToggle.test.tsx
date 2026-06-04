import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { AutonomyToggle, type AutonomyMode } from './AutonomyToggle'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('AutonomyToggle', () => {
  it('shows the current mode label', () => {
    wrap(<AutonomyToggle mode="ask" onChange={() => {}} />)
    expect(screen.getByText(/Ask/i)).toBeInTheDocument()
  })

  it('cycles ask -> auto-edit -> plan -> yolo -> ask on click', async () => {
    const seen: AutonomyMode[] = []
    const { rerender } = wrap(<AutonomyToggle mode="ask" onChange={(m) => seen.push(m)} />)
    await userEvent.click(screen.getByTestId('autonomy-toggle'))
    rerender(<LocaleProvider><AutonomyToggle mode="auto-edit" onChange={(m) => seen.push(m)} /></LocaleProvider>)
    await userEvent.click(screen.getByTestId('autonomy-toggle'))
    rerender(<LocaleProvider><AutonomyToggle mode="plan" onChange={(m) => seen.push(m)} /></LocaleProvider>)
    await userEvent.click(screen.getByTestId('autonomy-toggle'))
    rerender(<LocaleProvider><AutonomyToggle mode="yolo" onChange={(m) => seen.push(m)} /></LocaleProvider>)
    await userEvent.click(screen.getByTestId('autonomy-toggle'))
    expect(seen).toEqual(['auto-edit', 'plan', 'yolo', 'ask'])
  })
})
