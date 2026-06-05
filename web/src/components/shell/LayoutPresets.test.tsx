import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { LayoutPresets } from './LayoutPresets'

describe('LayoutPresets', () => {
  it('renders three presets and reports the chosen one', () => {
    const onChange = vi.fn()
    render(<LayoutPresets value="balanced" onChange={onChange} />)
    ;['balanced', 'focus', 'review'].forEach((p) => expect(screen.getByTestId(`preset-${p}`)).toBeInTheDocument())
    fireEvent.click(screen.getByTestId('preset-review'))
    expect(onChange).toHaveBeenCalledWith('review')
  })

  it('marks the active preset with aria-pressed=true', () => {
    render(<LayoutPresets value="focus" onChange={() => {}} />)
    expect(screen.getByTestId('preset-focus')).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByTestId('preset-balanced')).toHaveAttribute('aria-pressed', 'false')
  })
})
