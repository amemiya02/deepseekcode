import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import React from 'react'
import { LocaleProvider } from '../lib/i18n'
import { DiffView } from './DiffView'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

const PATCH = `@@ -1,2 +1,2 @@
-old line
+new line
 keep
@@ -5,1 +5,1 @@
-foo
+bar`

describe('DiffView', () => {
  it('renders both hunks via the PlainCode fallback', () => {
    wrap(<DiffView path="a.go" patch={PATCH} />)
    expect(screen.getAllByTestId('diff-hunk')).toHaveLength(2)
    expect(screen.getByText('new line')).toBeInTheDocument()
  })

  it('accept fires onHunk(index, true)', async () => {
    const calls: Array<{ index: number; accepted: boolean }> = []
    wrap(<DiffView path="a.go" patch={PATCH} onHunk={(index, accepted) => calls.push({ index, accepted })} />)
    await userEvent.click(screen.getAllByTestId('hunk-accept')[0])
    expect(calls).toEqual([{ index: 0, accepted: true }])
  })

  it('reject fires onHunk(index, false)', async () => {
    const calls: Array<{ index: number; accepted: boolean }> = []
    wrap(<DiffView path="a.go" patch={PATCH} onHunk={(index, accepted) => calls.push({ index, accepted })} />)
    await userEvent.click(screen.getAllByTestId('hunk-reject')[1])
    expect(calls).toEqual([{ index: 1, accepted: false }])
  })

  it('disables a hunk\'s buttons after a decision', async () => {
    wrap(<DiffView path="a.go" patch={PATCH} onHunk={vi.fn()} />)
    await userEvent.click(screen.getAllByTestId('hunk-accept')[0])
    expect(screen.getAllByTestId('hunk-accept')[0]).toBeDisabled()
  })

  it('hides per-hunk actions when readOnly', () => {
    wrap(<DiffView path="a.ts" patch={PATCH} readOnly />)
    expect(screen.queryByTestId('hunk-accept')).toBeNull()
    expect(screen.queryByTestId('hunk-reject')).toBeNull()
    expect(screen.getAllByTestId('diff-hunk').length).toBeGreaterThan(0)
  })

  it('renders as an island with +N −M counts in the header', () => {
    const { container } = wrap(<DiffView path="a.go" patch={PATCH} />)
    expect(container.querySelector('.island')).not.toBeNull()
    expect(screen.getByTestId('diff-added')).toHaveTextContent('+2')
    expect(screen.getByTestId('diff-removed')).toHaveTextContent('2')
  })
})
