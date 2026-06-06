import { useState } from 'react'
import type { DragEvent, ReactNode } from 'react'

export function AttachmentDrop({ onAttach, children }: { onAttach: (files: File[]) => void; children?: ReactNode }) {
  const [dragOver, setDragOver] = useState(false)
  const onDrop = (e: DragEvent<HTMLDivElement>) => { e.preventDefault(); setDragOver(false); const f = Array.from(e.dataTransfer.files); if (f.length) onAttach(f) }
  const onDragOver = (e: DragEvent<HTMLDivElement>) => { e.preventDefault(); setDragOver(true) }
  const onDragLeave = () => setDragOver(false)
  return (
    <div className={`attach ${dragOver ? 'attach--over' : ''}`} data-testid="attach-zone" onDrop={onDrop} onDragOver={onDragOver} onDragLeave={onDragLeave}>
      {children}
    </div>
  )
}
