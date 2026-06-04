import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { CommandPalette, type Command } from './CommandPalette'
import { LocaleProvider } from '../../lib/i18n'

function cmds(run = vi.fn()): Command[] {
  return [
    { id: 'new', title: 'New session', run },
    { id: 'theme', title: 'Switch theme', run },
    { id: 'settings', title: 'Open settings', run },
  ]
}

function renderPalette(props: Partial<React.ComponentProps<typeof CommandPalette>>) {
  return render(
    <LocaleProvider>
      <CommandPalette open={false} commands={cmds()} {...props} />
    </LocaleProvider>,
  )
}

describe('CommandPalette', () => {
  it('does not render the input when closed', () => {
    renderPalette({ open: false })
    expect(screen.queryByPlaceholderText(/Search commands/)).toBeNull()
  })

  it('renders all commands when open', () => {
    renderPalette({ open: true })
    expect(screen.getByText('New session')).toBeInTheDocument()
    expect(screen.getByText('Switch theme')).toBeInTheDocument()
  })

  it('filters commands by the query', async () => {
    const user = userEvent.setup()
    renderPalette({ open: true })
    await user.type(screen.getByPlaceholderText(/Search commands/), 'theme')
    expect(screen.getByText('Switch theme')).toBeInTheDocument()
    expect(screen.queryByText('New session')).toBeNull()
  })

  it('runs a command and closes when an item is selected', async () => {
    const user = userEvent.setup()
    const run = vi.fn()
    const onClose = vi.fn()
    renderPalette({ open: true, commands: cmds(run), onClose })
    await user.click(screen.getByText('New session'))
    expect(run).toHaveBeenCalledOnce()
    expect(onClose).toHaveBeenCalled()
  })

  it('closes on Escape', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    renderPalette({ open: true, onClose })
    await user.keyboard('{Escape}')
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })
})
