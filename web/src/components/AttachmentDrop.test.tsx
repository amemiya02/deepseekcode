import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import React from 'react'
import { LocaleProvider } from '../lib/i18n'
import { AttachmentDrop } from './AttachmentDrop'

function makeFile(name: string): File {
  return new File(['x'], name, { type: 'text/plain' })
}

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('AttachmentDrop', () => {
  it('drop fires onAttach with the dropped files', () => {
    const got: string[][] = []
    wrap(<AttachmentDrop onAttach={(files) => got.push(files.map((f) => f.name))} />)
    const zone = screen.getByTestId('attach-zone')
    fireEvent.drop(zone, { dataTransfer: { files: [makeFile('a.txt')], items: [], types: ['Files'] } })
    expect(got).toEqual([['a.txt']])
  })

  it('file input change fires onAttach', () => {
    const got: string[][] = []
    wrap(<AttachmentDrop onAttach={(files) => got.push(files.map((f) => f.name))} />)
    const input = screen.getByTestId('attach-input') as HTMLInputElement
    fireEvent.change(input, { target: { files: [makeFile('b.png')] } })
    expect(got).toEqual([['b.png']])
  })
})
