import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { ExtensionsSection } from './ExtensionsSection'

afterEach(() => vi.restoreAllMocks())

function mockFetch(map: Record<string, unknown>) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const url = typeof input === 'string' ? input : input.toString()
    const path = url.split('?')[0]
    const body = map[path] ?? { items: [] }
    return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
  })
}

describe('ExtensionsSection', () => {
  it('renders four tabs: MCP, Skills, Hooks, Memory', async () => {
    mockFetch({ '/v1/mcp': { items: [] } })
    render(<ExtensionsSection />)
    for (const name of [/mcp/i, /skills/i, /hooks/i, /memory/i]) {
      expect(screen.getByRole('tab', { name })).toBeInTheDocument()
    }
  })

  it('lists items for the active tab', async () => {
    mockFetch({ '/v1/mcp': { items: [{ id: 'fs', name: 'filesystem', enabled: true }] } })
    render(<ExtensionsSection />)
    await waitFor(() => expect(screen.getByText('filesystem')).toBeInTheDocument())
  })

  it('switching to Hooks fetches the hooks endpoint', async () => {
    const spy = mockFetch({
      '/v1/mcp': { items: [] },
      '/v1/hooks': { items: [{ id: 'h1', name: 'PreToolUse', enabled: true }] },
    })
    render(<ExtensionsSection />)
    await userEvent.click(screen.getByRole('tab', { name: /hooks/i }))
    await waitFor(() => {
      expect(spy).toHaveBeenCalledWith('/v1/hooks', expect.anything())
      expect(screen.getByText('PreToolUse')).toBeInTheDocument()
    })
  })
})
