import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { ContextPills } from './ContextPills'

describe('ContextPills', () => {
  it('renders one pill per item', () => {
    render(<LocaleProvider><ContextPills items={[{ id: 'a', label: 'main.go' }, { id: 'b', label: 'src/' }]} onRemove={() => {}} /></LocaleProvider>)
    expect(screen.getByText('main.go')).toBeInTheDocument()
    expect(screen.getByText('src/')).toBeInTheDocument()
  })

  it('remove button fires onRemove(id)', async () => {
    const removed: string[] = []
    render(<LocaleProvider><ContextPills items={[{ id: 'a', label: 'main.go' }]} onRemove={(id) => removed.push(id)} /></LocaleProvider>)
    await userEvent.click(screen.getByTestId('pill-remove-a'))
    expect(removed).toEqual(['a'])
  })
})
