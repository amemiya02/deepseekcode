import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { FileMenu } from './FileMenu'
import type { FileEntry } from '../lib/api'

const entries: FileEntry[] = [
  { name: 'src', path: 'src', is_dir: true },
  { name: 'main.go', path: 'main.go', is_dir: false },
]

describe('FileMenu', () => {
  it('renders entries; a directory shows a trailing slash', () => {
    render(<FileMenu items={entries} activeIndex={0} onPick={() => {}} onHover={() => {}} />)
    expect(screen.getByText('src/')).toBeInTheDocument()
    expect(screen.getByText('main.go')).toBeInTheDocument()
  })

  it('mousedown picks the entry', () => {
    const picked: string[] = []
    render(<FileMenu items={entries} activeIndex={0} onPick={(e) => picked.push(e.name)} onHover={() => {}} />)
    fireEvent.mouseDown(screen.getByText('main.go'))
    expect(picked).toEqual(['main.go'])
  })
})
