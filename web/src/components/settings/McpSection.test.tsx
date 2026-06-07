import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, afterEach } from 'vitest'
import React from 'react'
import { LocaleProvider } from '../../lib/i18n'
import { McpSection } from './McpSection'

afterEach(() => vi.restoreAllMocks())

function mockFetch(handler: (url: string, init?: RequestInit) => unknown) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = typeof input === 'string' ? input : input.toString()
    return new Response(JSON.stringify(handler(url, init)), { status: 200, headers: { 'Content-Type': 'application/json' } })
  })
}
const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('McpSection', () => {
  it('lists configured servers with command + enabled toggle', async () => {
    mockFetch(() => ({ items: [{ id: 'cg', name: 'cg', enabled: true, transport: 'stdio', command: 'codegraph' }] }))
    wrap(<McpSection />)
    await waitFor(() => expect(screen.getByText('cg')).toBeInTheDocument())
    expect(screen.getByText(/codegraph/)).toBeInTheDocument()
    expect(screen.getByTestId('mcp-toggle-cg')).toBeInTheDocument()
  })

  it('shows the empty state when no servers', async () => {
    mockFetch(() => ({ items: [] }))
    wrap(<McpSection />)
    await waitFor(() => expect(screen.getByText('No MCP servers configured.')).toBeInTheDocument())
  })

  it('renders the MCP discovery search input', async () => {
    mockFetch(() => ({ items: [] }))
    wrap(<McpSection />)
    await waitFor(() => expect(screen.getByText('No MCP servers configured.')).toBeInTheDocument())
    expect(screen.getByText('Discover tools')).toBeInTheDocument()
    expect(screen.getByLabelText('Search MCP tools')).toBeInTheDocument()
  })

  it('opens the add sub-view and posts a new server', async () => {
    const posted: RequestInit[] = []
    mockFetch((_url, init) => {
      if (init?.method === 'POST') { posted.push(init); return { items: [{ id: 'new', name: 'new', enabled: true, transport: 'stdio', command: 'x' }] } }
      return { items: [] }
    })
    wrap(<McpSection />)
    await userEvent.click(await screen.findByTestId('mcp-add'))
    await userEvent.type(screen.getByTestId('mcp-field-name'), 'new')
    await userEvent.type(screen.getByTestId('mcp-field-command'), 'x')
    await userEvent.click(screen.getByTestId('mcp-save'))
    await waitFor(() => expect(posted.length).toBe(1))
    expect(JSON.parse(posted[0].body as string)).toMatchObject({ name: 'new', command: 'x' })
  })
})
