import { useEffect, type ReactNode } from 'react'
import { useThemeStore } from '../../lib/theme/store'
import { buildTokens } from '../../lib/theme/tokens'

// ThemeProvider applies the active theme settings to the document root as CSS
// custom properties + data-* attributes. It re-applies whenever the Zustand
// store changes (useThemeStore subscribes this component to setting updates).
export function ThemeProvider({ children }: { children: ReactNode }) {
  const settings = useThemeStore((s) => s.settings)

  useEffect(() => {
    const tokens = buildTokens(settings)
    const root = document.documentElement
    for (const [k, v] of Object.entries(tokens)) {
      root.style.setProperty(k, v)
    }
    root.setAttribute('data-theme', settings.theme)
    root.setAttribute('data-mode', settings.mode)
    root.setAttribute('data-density', settings.density)
  }, [settings])

  return <>{children}</>
}
