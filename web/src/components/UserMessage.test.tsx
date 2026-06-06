import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { UserMessage } from './UserMessage'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('UserMessage', () => {
  it('renders the user text', () => {
    wrap(<UserMessage text="fix the nil deref" />)
    expect(screen.getByText('fix the nil deref')).toBeInTheDocument()
  })

  it('renders attached context pills', () => {
    wrap(<UserMessage text="see this" pills={['main.go', 'src/']} />)
    expect(screen.getByText('main.go')).toBeInTheDocument()
    expect(screen.getByText('src/')).toBeInTheDocument()
  })

  it('folds a large pasted body', () => {
    const big = Array.from({ length: 40 }, (_, i) => `l${i}`).join('\n')
    wrap(<UserMessage text={big} />)
    expect(screen.getByText(/Pasted 40 lines/)).toBeInTheDocument()
  })

  it('renders a user avatar and contained bubble', () => {
    wrap(<UserMessage text="hello" />)
    expect(screen.getByTestId('msg-avatar')).toBeInTheDocument()
    expect(screen.getByText('You')).toBeInTheDocument()
  })
})
