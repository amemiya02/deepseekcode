import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkspacePanel } from './WorkspacePanel'

function mockFetch(json: unknown) {
  return vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => json } as Response)
}

describe('WorkspacePanel', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('shows the Files tab by default with tree entries', async () => {
    vi.stubGlobal('fetch', mockFetch({ entries: [{ name: 'main.go', path: 'main.go', is_dir: false }] }))
    render(<WorkspacePanel />)
    await waitFor(() => expect(screen.getByText('main.go')).toBeInTheDocument())
  })

  it('switches to the Changed tab', async () => {
    vi.stubGlobal('fetch', mockFetch({ entries: [{ path: 'a.go', status: ' M', deleted: false }] }))
    render(<WorkspacePanel />)
    await userEvent.click(screen.getByTestId('tab-changed'))
    await waitFor(() => expect(screen.getByText('a.go')).toBeInTheDocument())
  })
})
