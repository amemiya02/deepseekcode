// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/FileMenu.tsx
// Presentational "@" file-reference dropdown. Reuses the .slashmenu container.
// FileEntry is the single Contract-1 type ({name,path,is_dir}), imported from api.ts
// (which re-exports it from workspace.ts).
import { useEffect, useRef } from 'react'
import { Folder, FileText } from 'lucide-react'
import type { FileEntry } from '../lib/api'

export function FileMenu({
  items, activeIndex, onPick, onHover,
}: {
  items: FileEntry[]
  activeIndex: number
  onPick: (e: FileEntry) => void
  onHover: (i: number) => void
}) {
  const activeRef = useRef<HTMLButtonElement>(null)
  useEffect(() => { activeRef.current?.scrollIntoView({ block: 'nearest' }) }, [activeIndex])
  return (
    <div className="slashmenu" role="listbox">
      {items.map((e, i) => (
        <button
          key={(e.is_dir ? 'd:' : 'f:') + e.path}
          ref={i === activeIndex ? activeRef : undefined}
          role="option"
          aria-selected={i === activeIndex}
          className={`slashmenu__item ${i === activeIndex ? 'slashmenu__item--active' : ''}`}
          onMouseDown={(ev) => { ev.preventDefault(); onPick(e) }}
          onMouseMove={() => onHover(i)}
        >
          {e.is_dir
            ? <Folder size={13} className="filemenu__icon filemenu__icon--dir" />
            : <FileText size={13} className="filemenu__icon" />}
          <span className="slashmenu__name slashmenu__name--file">{e.name}{e.is_dir ? '/' : ''}</span>
        </button>
      ))}
    </div>
  )
}
