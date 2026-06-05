import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { Panel, PanelHeader } from './Panel'

describe('Panel', () => {
  it('renders a header title, actions slot, and children', () => {
    render(<Panel><PanelHeader title="Workspace" actions={<button>x</button>} />body</Panel>)
    expect(screen.getByText('Workspace')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'x' })).toBeInTheDocument()
    expect(screen.getByText('body')).toBeInTheDocument()
  })
})
