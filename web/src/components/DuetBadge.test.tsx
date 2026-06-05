import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import React from 'react'
import { LocaleProvider } from '../lib/i18n'
import { DuetBadge } from './DuetBadge'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('DuetBadge', () => {
  it('renders the validated-by-pro state for allow', () => {
    wrap(<DuetBadge decision="allow" reason="looks safe" />)
    const badge = screen.getByTestId('duet-badge')
    expect(badge).not.toHaveClass('duet--warn')
    expect(screen.getByText('looks safe')).toBeInTheDocument()
  })
  it('renders the pushed-back state for deny', () => {
    wrap(<DuetBadge decision="deny" reason="out of scope" />)
    expect(screen.getByTestId('duet-badge')).toHaveClass('duet--warn')
    expect(screen.getByText('out of scope')).toBeInTheDocument()
  })
})
