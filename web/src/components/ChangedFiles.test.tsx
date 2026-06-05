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

  it('renders status badge with data-testid="change-status" and trimmed code', async () => {
    vi.stubGlobal('fetch', mockFetch([{ path: 'src/foo.ts', status: ' M', deleted: false }]))
    render(<ChangedFiles />)
    await waitFor(() => {
      const badge = screen.getByTestId('change-status')
      expect(badge).toBeInTheDocument()
      expect(badge.getAttribute('data-code')).toBe('M')
    })
  })

  it('splits path into dir and name parts', async () => {
    vi.stubGlobal('fetch', mockFetch([{ path: 'src/components/Foo.tsx', status: ' M', deleted: false }]))
    render(<ChangedFiles />)
    await waitFor(() => {
      expect(screen.getByText('src/components/')).toBeInTheDocument()
      expect(screen.getByText('Foo.tsx')).toBeInTheDocument()
    })
  })

  it('marks selected row with aria-current="true"', async () => {
    vi.stubGlobal('fetch', mockFetch([
      { path: 'a.go', status: ' M', deleted: false },
      { path: 'b.go', status: ' A', deleted: false },
    ]))
    render(<ChangedFiles selected="a.go" />)
    await waitFor(() => {
      // find the button that is the row for a.go
      const rows = screen.getAllByRole('button')
      const aRow = rows.find((r) => r.getAttribute('aria-current') === 'true')
      expect(aRow).toBeTruthy()
      const bRows = rows.filter((r) => r.getAttribute('aria-current') !== 'true')
      expect(bRows.length).toBeGreaterThan(0)
    })
  })

  it('shows empty state when no changes', async () => {
    vi.stubGlobal('fetch', mockFetch([]))
    render(<ChangedFiles />)
    await waitFor(() => expect(screen.getByText(/no changes/i)).toBeInTheDocument())
  })

  it('shows error state when fetch fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network error')))
    render(<ChangedFiles />)
    await waitFor(() => expect(screen.getByText(/network error/i)).toBeInTheDocument())
  })
})
