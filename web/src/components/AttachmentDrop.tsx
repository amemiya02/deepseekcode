import { useRef, useState } from 'react'
import type { ChangeEvent, DragEvent, ReactNode } from 'react'
import { Paperclip } from 'lucide-react'
import { useT } from '../lib/i18n'

export function AttachmentDrop({ onAttach, children }: { onAttach: (files: File[]) => void; children?: ReactNode }) {
  const t = useT()
  const [dragOver, setDragOver] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setDragOver(false)
    const files = Array.from(e.dataTransfer.files)
    if (files.length > 0) onAttach(files)
  }
  const onDragOver = (e: DragEvent<HTMLDivElement>) => { e.preventDefault(); setDragOver(true) }
  const onDragLeave = () => setDragOver(false)
  const onChange = (e: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? [])
    if (files.length > 0) onAttach(files)
    e.target.value = ''
  }

  return (
    <div
      className={`attach ${dragOver ? 'attach--over' : ''}`}
      data-testid="attach-zone"
      onDrop={onDrop}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
    >
      {children}
      <button type="button" className="attach__btn" onClick={() => inputRef.current?.click()} aria-label={t('composer.attach', 'Attach files')}>
        <Paperclip size={15} />
      </button>
      <input ref={inputRef} data-testid="attach-input" type="file" multiple hidden onChange={onChange} />
    </div>
  )
}
