import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { SddPanel } from './SddPanel'

describe('SddPanel', () => {
  it('shows a draft editor for a new requirement', () => {
    render(<SddPanel draft="" onChange={() => {}} onSubmit={() => {}} />)
    expect(screen.getByRole('textbox')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /draft|submit|generate|生成/i })).toBeInTheDocument()
  })
})
