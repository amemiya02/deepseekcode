import { useState, useRef, useCallback, useId, useEffect, useLayoutEffect, type CSSProperties } from 'react'
import { createPortal } from 'react-dom'
import { ChevronsUpDown, Check } from 'lucide-react'

export interface SelectOption { value: string; label: string }

const MENU_GAP = 4
const MENU_MAX_H = 280

export function BrandedSelect({
  value,
  options,
  onChange,
  ariaLabel,
  testid,
}: {
  value: string
  options: SelectOption[]
  onChange: (value: string) => void
  ariaLabel?: string
  testid?: string
}) {
  const [open, setOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState<number>(-1)
  // The menu is portaled to <body> and positioned with fixed coordinates derived
  // from the trigger rect. This is what fixes the settings bug: as an absolute
  // child it was clipped by the settings window's overflow:hidden (the upward
  // `bottom:100%` ran off the top edge), and the in-flow backdrop could be
  // covered by the window's backdrop-filter stacking context. A portal + a
  // document-level pointerdown listener make clipping and stuck-open impossible.
  const [pos, setPos] = useState<CSSProperties | null>(null)
  const menuId = useId()
  const listboxId = `${menuId}-listbox`
  const activeId = activeIndex >= 0 ? `${menuId}-opt-${activeIndex}` : undefined

  const current = options.find((o) => o.value === value)
  const currentIndex = options.findIndex((o) => o.value === value)

  const triggerRef = useRef<HTMLButtonElement>(null)
  const listboxRef = useRef<HTMLDivElement>(null)

  const pick = useCallback(
    (v: string) => {
      setOpen(false)
      setActiveIndex(-1)
      if (v !== value) onChange(v)
    },
    [value, onChange],
  )

  const openMenu = useCallback(() => {
    setOpen(true)
    setActiveIndex(currentIndex >= 0 ? currentIndex : 0)
  }, [currentIndex])

  const closeMenu = useCallback(() => {
    setOpen(false)
    setActiveIndex(-1)
  }, [])

  // Position the menu against the trigger, flipping upward only when there is
  // genuinely more room above. Runs before paint (useLayoutEffect) so the menu
  // never flashes at the wrong spot, and re-runs on scroll/resize while open.
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
      zIndex: 60, // above the settings overlay (z-index 50)
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

  // Move focus to the listbox once it is open AND positioned (the portal only
  // mounts when `pos` resolves). Re-running on reflow is harmless: focusing an
  // already-focused element is a no-op, so it never steals focus.
  useEffect(() => {
    if (open && pos) listboxRef.current?.focus()
  }, [open, pos])

  // Outside-close: any pointerdown outside the trigger AND the listbox closes the
  // menu. Capture phase + ref containment make this bulletproof regardless of the
  // portal target or stacking context (the old in-flow backdrop was not).
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
          setActiveIndex((i) => Math.min(i + 1, options.length - 1))
          break
        }
        case 'ArrowUp': {
          e.preventDefault()
          setActiveIndex((i) => Math.max(i - 1, 0))
          break
        }
        case 'Home': {
          e.preventDefault()
          setActiveIndex(0)
          break
        }
        case 'End': {
          e.preventDefault()
          setActiveIndex(options.length - 1)
          break
        }
        case 'Enter':
        case ' ': {
          e.preventDefault()
          if (activeIndex >= 0 && activeIndex < options.length) {
            pick(options[activeIndex].value)
          }
          break
        }
        case 'Escape': {
          e.preventDefault()
          closeMenu()
          break
        }
        case 'Tab': {
          // WAI-ARIA Listbox pattern: Tab MUST NOT be prevented — the browser
          // advances focus to the next focusable element. We close the popup
          // but let the event propagate so the user is never keyboard-trapped.
          closeMenu()
          break
        }
      }
    },
    [activeIndex, options, pick, closeMenu],
  )

  return (
    <div className="modelsw brandsel">
      <button
        ref={triggerRef}
        type="button"
        className="modelsw__trigger brandsel__trigger"
        data-testid={testid}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listboxId : undefined}
        aria-label={ariaLabel}
        onClick={() => (open ? closeMenu() : openMenu())}
        onKeyDown={handleTriggerKeyDown}
      >
        <span className="modelsw__label">{current?.label ?? value}</span>
        <ChevronsUpDown size={11} />
      </button>
      {open &&
        pos &&
        createPortal(
          /* eslint-disable-next-line jsx-a11y/no-noninteractive-element-to-interactive-role */
          <div
            ref={listboxRef}
            id={listboxId}
            className="modelsw__menu brandsel__menu"
            role="listbox"
            aria-label={ariaLabel}
            aria-activedescendant={activeId}
            tabIndex={-1}
            onKeyDown={handleListboxKeyDown}
            style={pos}
          >
            {options.map((o, i) => {
              const isSelected = o.value === value
              const isActive = i === activeIndex
              return (
                <div
                  key={o.value}
                  id={`${menuId}-opt-${i}`}
                  role="option"
                  aria-selected={isSelected}
                  tabIndex={isActive ? 0 : -1}
                  className={`modelsw__item${isSelected ? ' modelsw__item--current' : ''}${isActive ? ' modelsw__item--active' : ''}`}
                  onClick={() => pick(o.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      pick(o.value)
                    }
                    // Let the listbox handle all other keys via bubbling
                  }}
                  onMouseEnter={() => setActiveIndex(i)}
                >
                  <span className="modelsw__model">{o.label}</span>
                  {isSelected && <Check size={13} className="modelsw__check" />}
                </div>
              )
            })}
          </div>,
          document.body,
        )}
    </div>
  )
}
