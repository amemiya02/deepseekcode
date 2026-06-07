import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { ModeTabs } from './ModeTabs'

describe('ModeTabs', () => {
  it('renders the Code tab active by default', () => {
    render(<ModeTabs mode="code" onChange={() => {}} />)
    expect(screen.getByRole('tab', { name: /code/i })).toHaveAttribute('aria-selected', 'true')
  })

  it('renders a disabled Write tab with "soon" label', () => {
    render(<ModeTabs mode="code" onChange={() => {}} />)
    const writeTab = screen.getByRole('tab', { name: /write/i })
    expect(writeTab).toBeDisabled()
    expect(writeTab).toHaveAttribute('aria-selected', 'false')
  })

  it('renders a tablist with two tabs', () => {
    render(<ModeTabs mode="code" onChange={() => {}} />)
    const tablist = screen.getByRole('tablist')
    expect(tablist).toBeInTheDocument()
    expect(screen.getAllByRole('tab')).toHaveLength(2)
  })

  it('calls onChange when Code tab is clicked', () => {
    const onChange = vi.fn()
    render(<ModeTabs mode="code" onChange={onChange} />)
    screen.getByRole('tab', { name: /code/i }).click()
    expect(onChange).toHaveBeenCalledWith('code')
  })
})
