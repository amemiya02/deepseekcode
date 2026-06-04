import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FileTree } from './FileTree'
import type { FileEntry } from '../lib/workspace'

const entries: FileEntry[] = [
  { name: 'pkg', path: 'pkg', is_dir: true },
  { name: 'main.go', path: 'main.go', is_dir: false },
  { name: 'README.md', path: 'README.md', is_dir: false },
]

describe('FileTree', () => {
  it('renders entry names', () => {
    render(<FileTree entries={entries} />)
    expect(screen.getByText('main.go')).toBeInTheDocument()
    expect(screen.getByText('pkg/')).toBeInTheDocument()
  })

  it('filters by query', async () => {
    render(<FileTree entries={entries} />)
    await userEvent.type(screen.getByTestId('file-filter'), 'main')
    expect(screen.queryByText('README.md')).not.toBeInTheDocument()
    expect(screen.getByText('main.go')).toBeInTheDocument()
  })

  it('fires onOpen when a file is clicked', async () => {
    const onOpen = vi.fn()
    render(<FileTree entries={entries} onOpen={onOpen} />)
    await userEvent.click(screen.getByText('main.go'))
    expect(onOpen).toHaveBeenCalledWith('main.go')
  })

  it('fires onAddToChat from the add button', async () => {
    const onAddToChat = vi.fn()
    render(<FileTree entries={entries} onAddToChat={onAddToChat} />)
    await userEvent.click(screen.getByTestId('add-main.go'))
    expect(onAddToChat).toHaveBeenCalledWith(entries[1])
  })
})
