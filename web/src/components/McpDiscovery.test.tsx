import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { McpDiscovery } from './McpDiscovery'

describe('McpDiscovery', () => {
  it('renders a search box and tool results', () => {
    render(<McpDiscovery results={[{ server: 'fs', tool: 'read', desc: 'read a file' }]} onSearch={() => {}} onCall={() => {}} />)
    expect(screen.getByPlaceholderText(/search/i)).toBeInTheDocument()
    expect(screen.getByText(/read a file/i)).toBeInTheDocument()
  })

  it('calls onSearch when user types in the search box', () => {
    const onSearch = vi.fn()
    render(<McpDiscovery results={[]} onSearch={onSearch} onCall={() => {}} />)
    fireEvent.change(screen.getByPlaceholderText(/search/i), { target: { value: 'file' } })
    expect(onSearch).toHaveBeenCalledWith('file')
  })

  it('renders multiple tool results', () => {
    const results = [
      { server: 'fs', tool: 'read', desc: 'read a file' },
      { server: 'fs', tool: 'write', desc: 'write a file' },
      { server: 'git', tool: 'commit', desc: 'create a commit' },
    ]
    render(<McpDiscovery results={results} onSearch={() => {}} onCall={() => {}} />)
    expect(screen.getByText(/read a file/i)).toBeInTheDocument()
    expect(screen.getByText(/write a file/i)).toBeInTheDocument()
    expect(screen.getByText(/create a commit/i)).toBeInTheDocument()
  })

  it('shows server name for each result', () => {
    render(<McpDiscovery results={[{ server: 'git', tool: 'status', desc: 'git status' }]} onSearch={() => {}} onCall={() => {}} />)
    expect(screen.getByText('git')).toBeInTheDocument()
  })

  it('renders a call button for each result', () => {
    const onCall = vi.fn()
    render(<McpDiscovery results={[{ server: 'fs', tool: 'read', desc: 'read a file' }]} onSearch={() => {}} onCall={onCall} />)
    const callBtn = screen.getByRole('button', { name: /call/i })
    fireEvent.click(callBtn)
    expect(onCall).toHaveBeenCalledWith('fs', 'read')
  })

  it('renders an empty state when results is empty', () => {
    render(<McpDiscovery results={[]} onSearch={() => {}} onCall={() => {}} />)
    expect(screen.getByPlaceholderText(/search/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /call/i })).not.toBeInTheDocument()
  })
})
