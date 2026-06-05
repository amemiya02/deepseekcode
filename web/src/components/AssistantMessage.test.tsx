import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { AssistantMessage } from './AssistantMessage'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('AssistantMessage', () => {
  it('renders raw streaming text with a caret while streaming', () => {
    const { container } = wrap(<AssistantMessage text="typing" streaming />)
    expect(screen.getByText('typing')).toBeInTheDocument()
    expect(container.querySelector('.cursor')).toBeInTheDocument()
  })

  it('renders markdown when settled', () => {
    wrap(<AssistantMessage text={'I found **the** bug'} streaming={false} />)
    expect(screen.getByText('the')).toBeInTheDocument()
  })

  it('renders an assistant avatar and name', () => {
    wrap(<AssistantMessage text="hi" streaming={false} />)
    expect(screen.getByTestId('msg-avatar')).toBeInTheDocument()
    expect(screen.getByText('DeepSeek')).toBeInTheDocument()
  })
})
