import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { GoalPanel } from './GoalPanel'
import { LocaleProvider } from '../lib/i18n'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('GoalPanel', () => {
  it('shows pause/resume/complete for an active goal', () => {
    wrap(<GoalPanel goal={{ text: 'ship v1', state: 'active' }} onPause={() => {}} onResume={() => {}} onComplete={() => {}} />)
    expect(screen.getByRole('button', { name: /pause|暂停/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /complete|完成/i })).toBeInTheDocument()
  })

  it('shows resume/complete for a paused goal', () => {
    wrap(<GoalPanel goal={{ text: 'ship v1', state: 'paused' }} onPause={() => {}} onResume={() => {}} onComplete={() => {}} />)
    expect(screen.getByRole('button', { name: /resume|继续/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /complete|完成/i })).toBeInTheDocument()
  })

  it('hides controls when goal is completed', () => {
    wrap(<GoalPanel goal={{ text: 'ship v1', state: 'completed' }} onPause={() => {}} onResume={() => {}} onComplete={() => {}} />)
    expect(screen.queryByRole('button', { name: /pause|暂停/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /resume|继续/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /complete|完成/i })).not.toBeInTheDocument()
  })

  it('calls onPause when pause button is clicked', () => {
    const onPause = vi.fn()
    wrap(<GoalPanel goal={{ text: 'ship v1', state: 'active' }} onPause={onPause} onResume={() => {}} onComplete={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: /pause|暂停/i }))
    expect(onPause).toHaveBeenCalledOnce()
  })

  it('calls onResume when resume button is clicked', () => {
    const onResume = vi.fn()
    wrap(<GoalPanel goal={{ text: 'ship v1', state: 'paused' }} onPause={() => {}} onResume={onResume} onComplete={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: /resume|继续/i }))
    expect(onResume).toHaveBeenCalledOnce()
  })

  it('calls onComplete when complete button is clicked', () => {
    const onComplete = vi.fn()
    wrap(<GoalPanel goal={{ text: 'ship v1', state: 'active' }} onPause={() => {}} onResume={() => {}} onComplete={onComplete} />)
    fireEvent.click(screen.getByRole('button', { name: /complete|完成/i }))
    expect(onComplete).toHaveBeenCalledOnce()
  })

  it('displays the goal text', () => {
    wrap(<GoalPanel goal={{ text: 'ship v1', state: 'active' }} onPause={() => {}} onResume={() => {}} onComplete={() => {}} />)
    expect(screen.getByText('ship v1')).toBeInTheDocument()
  })

  it('shows the goal state indicator', () => {
    wrap(<GoalPanel goal={{ text: 'ship v1', state: 'active' }} onPause={() => {}} onResume={() => {}} onComplete={() => {}} />)
    expect(screen.getByText(/active|进行中/i)).toBeInTheDocument()
  })
})
