import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { PastedTextFold } from './PastedTextFold'

const BIG = Array.from({ length: 30 }, (_, i) => `line ${i}`).join('\n')

describe('PastedTextFold', () => {
  it('shows a folded summary with line count', () => {
    render(<LocaleProvider><PastedTextFold text={BIG} /></LocaleProvider>)
    expect(screen.getByText(/Pasted 30 lines/)).toBeInTheDocument()
    expect(screen.queryByText('line 29')).toBeNull()
  })

  it('expands to full text on click', async () => {
    render(<LocaleProvider><PastedTextFold text={BIG} /></LocaleProvider>)
    await userEvent.click(screen.getByTestId('fold-toggle'))
    expect(screen.getByText(/line 29/)).toBeInTheDocument()
  })
})
