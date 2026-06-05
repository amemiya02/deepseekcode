import { useState, useRef, useCallback, useId, useEffect } from 'react'
import { ChevronsUpDown, Check } from 'lucide-react'

export interface SelectOption { value: string; label: string }

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
  const menuId = useId()
  const listboxId = `${menuId}-listbox`
  const activeId = activeIndex >= 0 ? `${menuId}-opt-${activeIndex}` : undefined

  const current = options.find((o) => o.value === value)
  const currentIndex = options.findIndex((o) => o.value === value)

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

  // Move focus to the listbox whenever it opens so keyboard events are
  // received immediately without requiring a second interaction. This runs
  // after React has committed the listbox DOM node, so listboxRef is valid.
  useEffect(() => {
    if (open) {
      listboxRef.current?.focus()
    }
  }, [open])

  const closeMenu = useCallback(() => {
    setOpen(false)
    setActiveIndex(-1)
  }, [])

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
      {open && (
        <>
          <div className="modelsw__backdrop" onClick={closeMenu} />
          {/* eslint-disable-next-line jsx-a11y/interactive-supports-focus */}
          <div
            ref={listboxRef}
            id={listboxId}
            className="modelsw__menu brandsel__menu"
            role="listbox"
            aria-label={ariaLabel}
            aria-activedescendant={activeId}
            tabIndex={-1}
            onKeyDown={handleListboxKeyDown}
            // tabIndex={-1} allows programmatic focus from openMenu()
            // eslint-disable-next-line jsx-a11y/no-noninteractive-element-to-interactive-role
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
          </div>
        </>
      )}
    </div>
  )
}
