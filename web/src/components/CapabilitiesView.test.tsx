import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { CapabilitiesView } from './CapabilitiesView'

describe('CapabilitiesView', () => {
  it('lists feature flags from runtime info', () => {
    render(<CapabilitiesView caps={{ mcp: true, web: true, skills: false }} />)
    expect(screen.getByText(/mcp/i)).toBeInTheDocument()
    expect(screen.getByText(/skills/i)).toBeInTheDocument()
  })

  it('renders on/off chips for each flag', () => {
    render(<CapabilitiesView caps={{ mcp: true, web: false }} />)
    const chips = screen.getAllByText(/on|off/)
    expect(chips.length).toBeGreaterThanOrEqual(2)
  })
})
