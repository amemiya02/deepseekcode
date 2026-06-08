import { useState, useRef, useCallback, useId, useEffect, useLayoutEffect, type CSSProperties } from 'react'
import { createPortal } from 'react-dom'
import { GitBranch, ChevronsUpDown, Check } from 'lucide-react'
import styles from './BrandedSelect.module.css'

const MENU_GAP = 4
const MENU_MAX_H = 280

export function GitBranchPicker({
  current,
  branches,
  onSelect,
  compact = false,
}: {
  current: string
  branches: string[]
  onSelect: (branch: string) => void
  /** Compact mode for titlebar — smaller trigger, no min-width. */
  compact?: boolean
}) {
  const [open, setOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState<number>(-1)
  const [pos, setPos] = useState<CSSProperties | null>(null)
  const menuId = useId()
  const listboxId = `${menuId}-listbox`
  const activeId = activeIndex >= 0 ? `${menuId}-opt-${activeIndex}` : undefined

  const currentIndex = branches.indexOf(current)

  const triggerRef = useRef<HTMLButtonElement>(null)
  const listboxRef = useRef<HTMLDivElement>(null)

  const pick = useCallback(
    (v: string) => {
      setOpen(false)
      setActiveIndex(-1)
      if (v !== current) onSelect(v)
    },
    [current, onSelect],
  )

  const openMenu = useCallback(() => {
    setOpen(true)
    setActiveIndex(currentIndex >= 0 ? currentIndex : 0)
  }, [currentIndex])

  const closeMenu = useCallback(() => {
    setOpen(false)
    setActiveIndex(-1)
  }, [])

  const place = useCallback(() => {
    const el = triggerRef.current
    if (!el) return
    const r = el.getBoundingClientRect()
    const vh = window.innerHeight || 768
    const spaceBelow = vh - r.bottom
    const spaceAbove = r.top
    const up = spaceBelow < 220 && spaceAbove > spaceBelow
    const maxHeight = Math.max(120, Math.min(MENU_MAX_H, (up ? spaceAbove : spaceBelow) - MENU_GAP - 8))
    const style: CSSProperties = {
      position: 'fixed',
      left: Math.round(r.left),
      minWidth: Math.round(r.width),
      maxHeight,
      overflowY: 'auto',
      overscrollBehavior: 'contain',
      zIndex: 60,
    }
    if (up) {
      style.bottom = Math.round(vh - r.top + MENU_GAP)
      style.top = 'auto'
    } else {
      style.top = Math.round(r.bottom + MENU_GAP)
      style.bottom = 'auto'
    }
    setPos(style)
  }, [])

  useLayoutEffect(() => {
    if (!open) {
      setPos(null)
      return
    }
    place()
    const onReflow = () => place()
    window.addEventListener('scroll', onReflow, true)
    window.addEventListener('resize', onReflow)
    return () => {
      window.removeEventListener('scroll', onReflow, true)
      window.removeEventListener('resize', onReflow)
    }
  }, [open, place])

  useEffect(() => {
    if (open && pos) listboxRef.current?.focus()
  }, [open, pos])

  useEffect(() => {
    if (!open) return
    const onDown = (e: PointerEvent) => {
      const target = e.target as Node | null
      if (triggerRef.current?.contains(target) || listboxRef.current?.contains(target)) return
      closeMenu()
    }
    document.addEventListener('pointerdown', onDown, true)
    return () => document.removeEventListener('pointerdown', onDown, true)
  }, [open, closeMenu])

  const handleTriggerKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        openMenu()
      }
    },
    [openMenu],
  )

  const handleListboxKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      switch (e.key) {
        case 'ArrowDown': {
          e.preventDefault()
          setActiveIndex((i) => Math.min(i + 1, branches.length - 1))
          break
        }
        case 'ArrowUp': {
          e.preventDefault()
          setActiveIndex((i) => Math.max(i - 1, 0))
          break
        }
        case 'Enter':
        case ' ': {
          e.preventDefault()
          if (activeIndex >= 0 && activeIndex < branches.length) {
            pick(branches[activeIndex])
          }
          break
        }
        case 'Escape': {
          e.preventDefault()
          closeMenu()
          break
        }
        case 'Tab': {
          closeMenu()
          break
        }
      }
    },
    [activeIndex, branches, pick, closeMenu],
  )

  return (
    <div>
      <button
        ref={triggerRef}
        type="button"
        className={`${styles.trigger}${compact ? ` ${styles.triggerCompact}` : ''}`}
        data-testid="branch-trigger"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listboxId : undefined}
        onClick={() => (open ? closeMenu() : openMenu())}
        onKeyDown={handleTriggerKeyDown}
      >
        <GitBranch size={13} data-testid="git-branch-icon" />
        <span>{current}</span>
        <ChevronsUpDown size={11} className={styles.chevron} />
      </button>
      {open &&
        pos &&
        createPortal(
          /* eslint-disable-next-line jsx-a11y/no-noninteractive-element-to-interactive-role */
          <div
            ref={listboxRef}
            id={listboxId}
            className={styles.menu}
            role="listbox"
            aria-activedescendant={activeId}
            tabIndex={-1}
            onKeyDown={handleListboxKeyDown}
            style={pos}
          >
            {branches.map((b, i) => {
              const isSelected = b === current
              const isActive = i === activeIndex
              return (
                <div
                  key={b}
                  id={`${menuId}-opt-${i}`}
                  role="option"
                  aria-selected={isSelected}
                  tabIndex={isActive ? 0 : -1}
                  className={`${styles.option}${isSelected ? ` ${styles.optionSelected}` : ''}${isActive ? ` ${styles.optionActive}` : ''}`}
                  onClick={() => pick(b)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      pick(b)
                    }
                  }}
                  onMouseEnter={() => setActiveIndex(i)}
                >
                  <span>{b}</span>
                  {isSelected && <Check size={13} className={styles.check} />}
                </div>
              )
            })}
          </div>,
          document.body,
        )}
    </div>
  )
}
