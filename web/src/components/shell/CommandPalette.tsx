import { useEffect, useState } from 'react'
import { Command as CmdkCommand } from 'cmdk'
import { useT } from '../../lib/i18n'
import styles from './index.module.css'

export interface Command {
  id: string
  title: string
  hint?: string
  run: () => void
}

export interface CommandPaletteProps {
  open: boolean
  commands: Command[]
  onClose?: () => void
}

export function CommandPalette({ open, commands, onClose }: CommandPaletteProps) {
  const t = useT()
  const [query, setQuery] = useState('')

  // Reset the query each time the palette opens.
  useEffect(() => {
    if (open) setQuery('')
  }, [open])

  if (!open) return null

  const close = () => onClose?.()
  const runAndClose = (cmd: Command) => {
    cmd.run()
    close()
  }

  return (
    <>
      <div
        className={styles.paletteOverlay}
        role="button"
        tabIndex={-1}
        aria-label="Close"
        onClick={close}
      />
      <CmdkCommand
        className={styles.palette}
        label={t('app.openPalette')}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            e.preventDefault()
            close()
          }
        }}
      >
        <CmdkCommand.Input
          autoFocus
          value={query}
          onValueChange={setQuery}
          placeholder={t('palette.placeholder')}
          className={styles.paletteInput}
        />
        <CmdkCommand.List className={styles.paletteList}>
          <CmdkCommand.Empty className={styles.paletteEmpty}>{t('palette.empty')}</CmdkCommand.Empty>
          {commands.map((cmd) => (
            <CmdkCommand.Item
              key={cmd.id}
              value={cmd.title}
              onSelect={() => runAndClose(cmd)}
              className={styles.paletteItem}
            >
              <span>{cmd.title}</span>
              {cmd.hint && <span className={styles.paletteHint}>{cmd.hint}</span>}
            </CmdkCommand.Item>
          ))}
        </CmdkCommand.List>
      </CmdkCommand>
    </>
  )
}
