// CodeIsland — the reusable "obsidian island" shell for code & diff产物 (spec §3
// material rule). It renders the dark material in EVERY app mode by scoping the
// surface/ink/border tokens to the fixed --island-* palette (see app.css `.island`),
// so token-driven children (code card, diff hunks) go dark automatically. A brand
// header strip (saturated dot + label + right-aligned meta/actions) sits above the body.
import type { ReactNode } from 'react'

export interface CodeIslandProps {
  /** Left header label — language for code, path for diffs. */
  label?: ReactNode
  /** Right-aligned passive meta — e.g. diff +N −M counts. */
  meta?: ReactNode
  /** Right-aligned interactive controls — e.g. a Copy button. */
  actions?: ReactNode
  /** Extra class for the body wrapper. */
  bodyClassName?: string
  children?: ReactNode
}

export function CodeIsland({ label, meta, actions, bodyClassName, children }: CodeIslandProps) {
  const hasBar = label != null || meta != null || actions != null
  return (
    <div className="island" data-island data-testid="code-island">
      {hasBar && (
        <div className="island__bar">
          <span className="island__dot" aria-hidden="true" />
          {label != null && <span className="island__label">{label}</span>}
          {(meta != null || actions != null) && (
            <span className="island__right">
              {meta != null && <span className="island__meta">{meta}</span>}
              {actions}
            </span>
          )}
        </div>
      )}
      <div className={`island__body${bodyClassName ? ` ${bodyClassName}` : ''}`}>{children}</div>
    </div>
  )
}
