import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { ChangedFiles } from './ChangedFiles'

function mockFetch(entries: unknown[]) {
  return vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => ({ entries }),
  } as Response)
}

describe('ChangedFiles', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('renders changed rows fetched on mount', async () => {
    vi.stubGlobal('fetch', mockFetch([{ path: 'a.go', status: ' M', deleted: false }]))
    render(<ChangedFiles />)
    await waitFor(() => expect(screen.getByText('a.go')).toBeInTheDocument())
  })

  it('shows a deleted marker', async () => {
    vi.stubGlobal('fetch', mockFetch([{ path: 'gone.go', status: ' D', deleted: true }]))
    render(<ChangedFiles />)
    await waitFor(() => expect(screen.getByTestId('deleted-gone.go')).toBeInTheDocument())
  })

  it('re-fetches when refreshKey changes', async () => {
    const f = mockFetch([{ path: 'a.go', status: ' M', deleted: false }])
    vi.stubGlobal('fetch', f)
    const { rerender } = render(<ChangedFiles refreshKey={0} />)
    await waitFor(() => expect(f).toHaveBeenCalledTimes(1))
    rerender(<ChangedFiles refreshKey={1} />)
    await waitFor(() => expect(f).toHaveBeenCalledTimes(2))
  })
})
