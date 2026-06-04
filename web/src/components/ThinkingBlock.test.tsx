import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { ThinkingBlock } from './ThinkingBlock'

describe('ThinkingBlock', () => {
  it('is collapsed by default (body hidden)', () => {
    render(<LocaleProvider><ThinkingBlock text="reasoning here" /></LocaleProvider>)
    expect(screen.queryByText('reasoning here')).toBeNull()
  })

  it('expands on toggle click', async () => {
    render(<LocaleProvider><ThinkingBlock text="reasoning here" /></LocaleProvider>)
    await userEvent.click(screen.getByTestId('thinking-toggle'))
    expect(screen.getByText('reasoning here')).toBeInTheDocument()
  })
})
