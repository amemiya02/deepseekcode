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
  it('renders children inside the drop zone', () => {
    wrap(<AttachmentDrop onAttach={() => {}}><span data-testid="child">hi</span></AttachmentDrop>)
    expect(screen.getByTestId('attach-zone')).toBeTruthy()
    expect(screen.getByTestId('child')).toBeTruthy()
  })

  it('applies attach--over class on dragOver and removes it on dragLeave', () => {
    wrap(<AttachmentDrop onAttach={() => {}} />)
    const zone = screen.getByTestId('attach-zone')
    expect(zone.className).not.toContain('attach--over')
    fireEvent.dragOver(zone, { dataTransfer: { items: [], types: ['Files'] } })
    expect(zone.className).toContain('attach--over')
    fireEvent.dragLeave(zone)
    expect(zone.className).not.toContain('attach--over')
  })

  it('drop fires onAttach with the dropped files', () => {
    const got: string[][] = []
    wrap(<AttachmentDrop onAttach={(files) => got.push(files.map((f) => f.name))} />)
    const zone = screen.getByTestId('attach-zone')
    fireEvent.drop(zone, { dataTransfer: { files: [makeFile('a.txt')], items: [], types: ['Files'] } })
    expect(got).toEqual([['a.txt']])
  })

  it('does not render an inline attach button or file input', () => {
    wrap(<AttachmentDrop onAttach={() => {}} />)
    expect(document.querySelector('.attach__btn')).toBeNull()
    expect(document.querySelector('[data-testid="attach-input"]')).toBeNull()
  })
})
