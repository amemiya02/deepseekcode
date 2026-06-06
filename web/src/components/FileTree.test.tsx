import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FileTree } from './FileTree'
import type { FileEntry } from '../lib/workspace'
import * as workspace from '../lib/workspace'

const entries: FileEntry[] = [
  { name: 'pkg', path: 'pkg', is_dir: true },
  { name: 'main.go', path: 'main.go', is_dir: false },
  { name: 'README.md', path: 'README.md', is_dir: false },
]

describe('FileTree', () => {
  beforeEach(() => vi.restoreAllMocks())

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

  it('expands a directory and shows children on click', async () => {
    const childEntries: FileEntry[] = [{ name: 'util.go', path: 'pkg/util.go', is_dir: false }]
    vi.spyOn(workspace, 'fetchFiles').mockResolvedValue({ entries: childEntries })
    render(<FileTree entries={entries} />)
    await userEvent.click(screen.getByTestId('dir-pkg'))
    await waitFor(() => expect(screen.getByText('util.go')).toBeInTheDocument())
    expect(workspace.fetchFiles).toHaveBeenCalledWith('pkg')
  })

  it('collapses a directory on second click', async () => {
    const childEntries: FileEntry[] = [{ name: 'util.go', path: 'pkg/util.go', is_dir: false }]
    vi.spyOn(workspace, 'fetchFiles').mockResolvedValue({ entries: childEntries })
    render(<FileTree entries={entries} />)
    await userEvent.click(screen.getByTestId('dir-pkg'))
    await waitFor(() => expect(screen.getByText('util.go')).toBeInTheDocument())
    // Second click collapses.
    await userEvent.click(screen.getByTestId('dir-pkg'))
    expect(screen.queryByText('util.go')).not.toBeInTheDocument()
  })

  it('does NOT call onOpen when a directory is clicked', async () => {
    const onOpen = vi.fn()
    vi.spyOn(workspace, 'fetchFiles').mockResolvedValue({ entries: [] })
    render(<FileTree entries={entries} onOpen={onOpen} />)
    await userEvent.click(screen.getByTestId('dir-pkg'))
    expect(onOpen).not.toHaveBeenCalled()
  })
})
