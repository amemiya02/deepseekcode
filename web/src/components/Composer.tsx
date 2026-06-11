// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/Composer.tsx
// Assembles the input experience: textarea with "/" (slash) and trailing-"@" (file)
// trigger detection, ContextPills, ModeMenu, ModelSwitcher, EffortSwitcher,
// SendStopButton, AttachmentDrop, and long-paste folding.
// Leaf-testable: commands arrive as a prop; files come from workspace.fetchFiles (Contract 1).
import { useEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { Paperclip } from 'lucide-react'
import { fetchFiles } from '../lib/workspace'
import type { FileEntry, ModelInfo } from '../lib/api'
import { useT } from '../lib/i18n'
import { ORDER, type AutonomyMode } from '../lib/autonomy'
import { SlashMenu, type SlashCommand } from './SlashMenu'
import { FileMenu } from './FileMenu'
import { ContextPills, type ContextPill } from './ContextPills'
import { ModelSwitcher } from './ModelSwitcher'
import { EffortSwitcher } from './EffortSwitcher'
import { ModeMenu } from './ModeMenu'

import { SendStopButton } from './SendStopButton'
import { AttachmentDrop } from './AttachmentDrop'

export interface ComposerPayload {
  text: string
  mode: AutonomyMode
  pills: string[]
  files: File[]
}

export function Composer({
  streaming, mode, commands, onSend, onCancel, onModeChange, disabled = false, draft,
  models = [], activeModel = '', effort = 'medium', effortLevels = [], onModelChange, onEffortChange,
}: {
  streaming: boolean
  mode: AutonomyMode
  commands: SlashCommand[]
  onSend: (payload: ComposerPayload) => void
  onCancel: () => void
  onModeChange: (mode: AutonomyMode) => void
  disabled?: boolean
  /** External content to append to the draft (e.g. from add-to-chat). */
  draft?: string
  models?: ModelInfo[]
  activeModel?: string
  effort?: string
  effortLevels?: string[]
  onModelChange?: (id: string, provider?: string) => void
  onEffortChange?: (level: string) => void
}) {
  const t = useT()
  const [text, setText] = useState('')

  // When an external draft is injected (e.g. via add-to-chat), append it once.
  const prevDraftRef = useRef<string | undefined>(undefined)
  useEffect(() => {
    if (draft !== undefined && draft !== prevDraftRef.current) {
      prevDraftRef.current = draft
      setText((prev) => (prev ? prev + '\n\n' + draft : draft))
      // Focus so the user can see the appended content immediately.
      taRef.current?.focus()
    }
  }, [draft])
  const [pills, setPills] = useState<ContextPill[]>([])
  const [files, setFiles] = useState<File[]>([])
  const [active, setActive] = useState(0)
  const [entries, setEntries] = useState<FileEntry[]>([])
  const taRef = useRef<HTMLTextAreaElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  // IME composition tracking. WebKit (the macOS desktop webview) fires the
  // candidate-confirming Enter's keydown AFTER compositionend with
  // isComposing === false, so the isComposing check alone misses it. We track
  // the composition state ourselves and remember WHEN it ended: an Enter that
  // lands within a few ms of compositionend is the confirm keystroke, not a
  // send. (A human pressing Enter twice is always far slower than 75 ms.)
  const composingRef = useRef(false)
  const compositionEndAtRef = useRef(-Infinity)

  const onPickFiles = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = Array.from(e.target.files ?? [])
    if (f.length) setFiles((p) => [...p, ...f])
    e.target.value = ''
  }

  // Slash query: whole-input "/token" with no whitespace yet.
  const slashQuery = useMemo(() => {
    if (!text.startsWith('/') || /\s/.test(text)) return null
    return text.slice(1).toLowerCase()
  }, [text])
  const slashMatches = useMemo(
    () => (slashQuery === null ? [] : commands.filter((c) => c.name.toLowerCase().includes(slashQuery)).slice(0, 8)),
    [slashQuery, commands],
  )

  // @ file reference: trailing "@token".
  const atFrag = useMemo(() => {
    const m = /(?:^|\s)@([^\s]*)$/.exec(text)
    return m ? m[1].toLowerCase() : null
  }, [text])

  useEffect(() => {
    if (atFrag === null) { setEntries([]); return }
    let live = true
    fetchFiles('').then((r) => { if (live) setEntries(r.entries) }).catch(() => {})
    return () => { live = false }
  }, [atFrag === null])

  const atMatches = useMemo(
    () => (atFrag === null ? [] : entries.filter((e) => e.name.toLowerCase().includes(atFrag)).slice(0, 10)),
    [atFrag, entries],
  )

  const menuMode: 'slash' | 'at' | null =
    slashMatches.length > 0 ? 'slash' : atMatches.length > 0 ? 'at' : null
  const count = menuMode === 'slash' ? slashMatches.length : menuMode === 'at' ? atMatches.length : 0

  useEffect(() => { setActive(0) }, [slashQuery, atFrag])

  const pickCommand = (c: SlashCommand) => setText('/' + c.name + ' ')
  const pickEntry = (e: FileEntry) => {
    setPills((prev) => (prev.some((p) => p.id === e.path) ? prev : [...prev, { id: e.path, label: e.name + (e.is_dir ? '/' : '') }]))
    // strip the trailing "@frag" token
    setText((prev) => prev.replace(/(?:^|\s)@[^\s]*$/, (m) => (m.startsWith(' ') ? ' ' : '')))
  }
  const pickActive = () => {
    if (menuMode === 'slash') pickCommand(slashMatches[active])
    else if (menuMode === 'at') pickEntry(atMatches[active])
  }

  const submit = () => {
    if (disabled) return
    const body = text.trim()
    if (!body && pills.length === 0 && files.length === 0) return
    onSend({ text: body, mode, pills: pills.map((p) => p.label), files })
    setText('')
    setPills([])
    setFiles([])
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    // IME composition guard: never act on a keystroke that drives the IME.
    // keyCode 229 covers Chrome-family; the composingRef + the post-compositionend
    // window covers WebKit, where the confirming Enter arrives after
    // compositionend with isComposing already false.
    if (e.nativeEvent.isComposing || e.keyCode === 229 || composingRef.current) return
    if (e.key === 'Enter' && e.timeStamp - compositionEndAtRef.current < 75) return

    if (e.key === 'Tab' && e.shiftKey) {
      e.preventDefault()
      onModeChange(ORDER[(ORDER.indexOf(mode) + 1) % ORDER.length])
      return
    }
    if (menuMode) {
      if (e.key === 'ArrowDown') { e.preventDefault(); setActive((i) => (i + 1) % count); return }
      if (e.key === 'ArrowUp') { e.preventDefault(); setActive((i) => (i - 1 + count) % count); return }
      if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); pickActive(); return }
    }
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit() }
    if (e.key === 'Escape' && streaming) { e.preventDefault(); onCancel() }
  }

  return (
    <div className="composer-wrap">
      {menuMode === 'slash' && <SlashMenu items={slashMatches} activeIndex={active} onPick={pickCommand} onHover={setActive} />}
      {menuMode === 'at' && <FileMenu items={atMatches} activeIndex={active} onPick={pickEntry} onHover={setActive} />}
      <ContextPills items={pills} onRemove={(id) => setPills((prev) => prev.filter((p) => p.id !== id))} />
      {files.length > 0 && (
        <div className="composer__files" data-testid="composer-files">
          {files.map((f, i) => (
            <span key={i} className="composer__file-chip">
              <span className="composer__file-name">{f.name}</span>
              <button
                type="button"
                className="composer__file-remove"
                data-testid={`remove-file-${i}`}
                aria-label={`Remove ${f.name}`}
                onClick={() => setFiles((prev) => prev.filter((_, j) => j !== i))}
              >
                ×
              </button>
            </span>
          ))}
        </div>
      )}
      <AttachmentDrop onAttach={(f) => setFiles((prev) => [...prev, ...f])}>
        <textarea
          ref={taRef}
          data-testid="composer-input"
          className="composer__input"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKeyDown}
          onCompositionStart={() => { composingRef.current = true }}
          onCompositionEnd={(e) => { composingRef.current = false; compositionEndAtRef.current = e.timeStamp }}
          placeholder={t('composer.placeholder', 'Ask, plan, or build…')}
          rows={1}
          disabled={disabled}
        />
      </AttachmentDrop>
      {streaming && text.trim().length > 0 && (
        <div className="composer__steer-hint" data-testid="composer-steer-hint">
          {t('composer.steerHint', 'Enter to steer · Esc to stop')}
        </div>
      )}
      <div className="composer-bar">
        <div className="composer-bar__left">
          <button
            type="button"
            className="composer-bar__btn"
            data-testid="attach-btn"
            onClick={() => fileRef.current?.click()}
            aria-label={t('composer.attach', 'Attach files')}
          >
            <Paperclip size={15} />
          </button>
          <input ref={fileRef} data-testid="attach-input" type="file" multiple hidden onChange={onPickFiles} />
          {models.length > 0 && onModelChange && (
            <ModelSwitcher label={activeModel || t('composer.model', 'Model')} activeId={activeModel} models={models} onPick={onModelChange} />
          )}
          {onEffortChange && (
            <EffortSwitcher levels={effortLevels} current={effort} disabled={streaming} onPick={onEffortChange} />
          )}
          <ModeMenu mode={mode} onChange={onModeChange} />
        </div>
        <SendStopButton
          streaming={streaming}
          disabled={disabled || (!text.trim() && pills.length === 0 && files.length === 0)}
          onSend={submit}
          onCancel={onCancel}
        />
      </div>
    </div>
  )
}
