import { render, screen, act } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'

// Mock @monaco-editor/react so the heavy module is never loaded in jsdom; assert
// our wrapper passes value/language through to the mocked editor host.
vi.mock('@monaco-editor/react', () => ({
  default: ({ value, language }: { value: string; language?: string }) => (
    <div data-testid="monaco-host" data-language={language}>
      {value}
    </div>
  ),
  DiffEditor: ({ modified, language }: { modified: string; language?: string }) => (
    <div data-testid="monaco-diff-host" data-language={language}>
      {modified}
    </div>
  ),
  loader: { init: () => Promise.resolve({}) },
}))

import { MonacoMount, PlainCode } from './monaco'

describe('monaco seam', () => {
  it('PlainCode fallback renders the raw code', () => {
    render(<PlainCode value={'const x = 1'} />)
    expect(screen.getByText('const x = 1')).toBeInTheDocument()
  })

  it('MonacoMount renders the (mocked) editor host with value + language', async () => {
    await act(async () => {
      render(<MonacoMount value={'let y = 2'} language="ts" />)
    })
    const host = screen.getByTestId('monaco-host')
    expect(host).toHaveTextContent('let y = 2')
    expect(host.getAttribute('data-language')).toBe('ts')
  })
})
