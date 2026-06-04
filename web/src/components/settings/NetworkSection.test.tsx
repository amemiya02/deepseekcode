import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { NetworkSection } from './NetworkSection'

describe('NetworkSection', () => {
  it('offers the four proxy modes', () => {
    render(<NetworkSection />)
    const sel = screen.getByLabelText(/proxy mode/i) as HTMLSelectElement
    const values = Array.from(sel.options).map((o) => o.value)
    expect(values).toEqual(expect.arrayContaining(['auto', 'env', 'custom', 'off']))
  })

  it('reveals the URL + no_proxy fields only in custom mode', async () => {
    render(<NetworkSection />)
    await userEvent.selectOptions(screen.getByLabelText(/proxy mode/i), 'custom')
    expect(screen.getByLabelText(/proxy url/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/no_proxy/i)).toBeInTheDocument()
  })

  it('shows the non-UTF-8/CJK round-trip note', () => {
    render(<NetworkSection />)
    expect(screen.getByText(/GBK|GB18030|non-UTF-8/i)).toBeInTheDocument()
  })
})
