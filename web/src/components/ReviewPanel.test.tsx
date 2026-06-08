import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import React from 'react'
import { LocaleProvider } from '../lib/i18n'
import { ReviewPanel } from './ReviewPanel'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

// Mock the workspace lib so network is never hit
vi.mock('../lib/workspace', () => ({
  fetchChanged: vi.fn(),
  fetchDiff: vi.fn(),
  fetchFile: vi.fn(),
}))

import { fetchChanged, fetchDiff, fetchFile } from '../lib/workspace'

const mockFetchChanged = fetchChanged as ReturnType<typeof vi.fn>
const mockFetchDiff = fetchDiff as ReturnType<typeof vi.fn>
const mockFetchFile = fetchFile as ReturnType<typeof vi.fn>

const SAMPLE_PATCH = `@@ -1,3 +1,4 @@
 package main

+// added line
 func main() {}
`

describe('ReviewPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchChanged.mockResolvedValue({
      entries: [{ path: 'src/main.go', status: ' M', deleted: false }],
    })
    mockFetchDiff.mockResolvedValue({ path: 'src/main.go', patch: SAMPLE_PATCH })
    mockFetchFile.mockResolvedValue({
      path: 'src/main.go',
      content: 'package main\n\nfunc main() {}',
      binary: false,
      truncated: false,
    })
  })

  it('shows placeholder when nothing is selected', async () => {
    wrap(<ReviewPanel />)
    await waitFor(() => expect(screen.getByTestId('review-placeholder')).toBeInTheDocument())
    expect(screen.queryByTestId('diff-hunk')).toBeNull()
  })

  it('shows diff-hunk after clicking a row that has a non-empty patch', async () => {
    wrap(<ReviewPanel />)
    // wait for the changed file row to appear
    await waitFor(() => expect(screen.getByText('main.go')).toBeInTheDocument())
    // click the row
    fireEvent.click(screen.getByRole('button', { name: /main\.go/ }))
    // diff hunk should appear
    await waitFor(() => expect(screen.getAllByTestId('diff-hunk').length).toBeGreaterThan(0))
    expect(screen.queryByTestId('review-placeholder')).toBeNull()
  })

  it('falls back to CodeViewer (code-fallback) when patch is empty', async () => {
    mockFetchDiff.mockResolvedValue({ path: 'src/main.go', patch: '' })
    wrap(<ReviewPanel />)
    await waitFor(() => expect(screen.getByText('main.go')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /main\.go/ }))
    // Suspense fallback renders data-testid="code-fallback"
    await waitFor(() => expect(screen.getByTestId('code-fallback')).toBeInTheDocument())
    expect(screen.queryByTestId('diff-hunk')).toBeNull()
  })

  it('renders the review-hero container', async () => {
    wrap(<ReviewPanel />)
    await waitFor(() => expect(screen.getByTestId('review-hero')).toBeInTheDocument())
  })

  it('exposes per-file approve and reject', async () => {
    wrap(<ReviewPanel />)
    await waitFor(() => expect(screen.getByText('main.go')).toBeInTheDocument())
    expect(screen.getByTestId('approve-src/main.go')).toBeInTheDocument()
    expect(screen.getByTestId('reject-src/main.go')).toBeInTheDocument()
  })

  it('clicking approve marks the file as approved', async () => {
    wrap(<ReviewPanel />)
    await waitFor(() => expect(screen.getByText('main.go')).toBeInTheDocument())
    fireEvent.click(screen.getByTestId('approve-src/main.go'))
    await waitFor(() => {
      expect(screen.getByTestId('file-status-src/main.go')).toHaveTextContent(/approved/i)
    })
  })

  it('clicking reject marks the file as rejected', async () => {
    wrap(<ReviewPanel />)
    await waitFor(() => expect(screen.getByText('main.go')).toBeInTheDocument())
    fireEvent.click(screen.getByTestId('reject-src/main.go'))
    await waitFor(() => {
      expect(screen.getByTestId('file-status-src/main.go')).toHaveTextContent(/rejected/i)
    })
  })
})

test('ReviewPanel exposes a draggable list|diff splitter', () => {
  render(<ReviewPanel refreshKey={0} />)
  const splitter = screen.getByTestId('review-splitter')
  expect(splitter).toHaveAttribute('role', 'separator')
  // dragging right widens the list column without throwing
  fireEvent.mouseDown(splitter, { clientX: 200 })
  fireEvent.mouseMove(window, { clientX: 260 })
  fireEvent.mouseUp(window)
  expect(splitter).toBeInTheDocument()
})
