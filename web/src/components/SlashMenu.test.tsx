import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { SlashMenu, type SlashCommand } from './SlashMenu'

const cmds: SlashCommand[] = [
  { name: 'review', description: 'Review the diff', kind: 'builtin' },
  { name: 'model', description: 'Switch model', kind: 'builtin' },
]

describe('SlashMenu', () => {
  it('renders one row per command with the leading slash', () => {
    render(<LocaleProvider><SlashMenu items={cmds} activeIndex={0} onPick={() => {}} onHover={() => {}} /></LocaleProvider>)
    expect(screen.getByText('/review')).toBeInTheDocument()
    expect(screen.getByText('/model')).toBeInTheDocument()
  })

  it('marks the active item as selected', () => {
    render(<LocaleProvider><SlashMenu items={cmds} activeIndex={1} onPick={() => {}} onHover={() => {}} /></LocaleProvider>)
    const options = screen.getAllByRole('option')
    expect(options[1]).toHaveAttribute('aria-selected', 'true')
  })

  it('mousedown picks the command', () => {
    const picked: string[] = []
    render(<LocaleProvider><SlashMenu items={cmds} activeIndex={0} onPick={(c) => picked.push(c.name)} onHover={() => {}} /></LocaleProvider>)
    fireEvent.mouseDown(screen.getByText('/review'))
    expect(picked).toEqual(['review'])
  })
})
