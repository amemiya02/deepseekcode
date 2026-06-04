import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { EffortSwitcher } from './EffortSwitcher'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('EffortSwitcher', () => {
  it('renders nothing when there are no levels', () => {
    const { container } = wrap(<EffortSwitcher levels={[]} current="auto" disabled={false} onPick={() => {}} />)
    expect(container.firstChild).toBeNull()
  })

  it('opens and lists the levels', async () => {
    wrap(<EffortSwitcher levels={['auto', 'low', 'high']} current="auto" disabled={false} onPick={() => {}} />)
    await userEvent.click(screen.getByTestId('effort-trigger'))
    expect(await screen.findByText('high')).toBeInTheDocument()
  })

  it('picking a different level fires onPick(level)', async () => {
    const picked: string[] = []
    wrap(<EffortSwitcher levels={['auto', 'high']} current="auto" disabled={false} onPick={(l) => picked.push(l)} />)
    await userEvent.click(screen.getByTestId('effort-trigger'))
    await userEvent.click(await screen.findByText('high'))
    expect(picked).toEqual(['high'])
  })
})
