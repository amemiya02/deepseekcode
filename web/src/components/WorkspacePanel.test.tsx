import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { WorkspacePanel } from './WorkspacePanel'

function mockFetch(json: unknown) {
  return vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => json } as Response)
}

describe('WorkspacePanel', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('shows the Changed tab by default with changed entries', async () => {
    vi.stubGlobal('fetch', mockFetch({ entries: [{ path: 'a.go', status: ' M', deleted: false }] }))
    render(<WorkspacePanel />)
    // The Changed tab is selected and the changed file renders.
    expect(screen.getByTestId('tab-changed')).toHaveAttribute('aria-selected', 'true')
    await waitFor(() => expect(screen.getByText('a.go')).toBeInTheDocument())
  })

  it('never requests /v1/files with an empty path (Files tab removed)', async () => {
    const fetchMock = mockFetch({ entries: [] })
    vi.stubGlobal('fetch', fetchMock)
    render(<WorkspacePanel />)
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const urls = fetchMock.mock.calls.map((c) => String(c[0]))
    // Only /v1/changed should be hit; no /v1/files?path= (the 400 noise).
    expect(urls.some((u) => u.includes('/v1/changed'))).toBe(true)
    expect(urls.some((u) => u.includes('/v1/files'))).toBe(false)
  })
})
