import { useEffect, useMemo, useState } from 'react'
import { LocaleProvider, useLocale, useT } from './lib/i18n'
import { ThemeProvider } from './components/shell/ThemeProvider'
import { ErrorBoundary } from './components/shell/ErrorBoundary'
import { AppShell } from './components/shell/AppShell'
import { TitleBar } from './components/shell/TitleBar'
import { CommandPalette, type Command } from './components/shell/CommandPalette'
import { Toasts } from './components/shell/Toasts'
import { isCmdK } from './lib/shortcuts'
import { setThemeSettings, useThemeStore } from './lib/theme/store'
import styles from './components/shell/index.module.css'

// AppInner lives under LocaleProvider so hooks (useT/useLocale) have their context.
function AppInner() {
  const t = useT()
  const { locale, setLocale } = useLocale()
  const [paletteOpen, setPaletteOpen] = useState(false)

  // Window-level Cmd/Ctrl+K opens the palette.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (isCmdK(e)) {
        e.preventDefault()
        setPaletteOpen(true)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const commands = useMemo<Command[]>(
    () => [
      { id: 'new-session', title: t('app.newSession'), run: () => {} },
      {
        id: 'toggle-mode',
        title: t('titlebar.toggleMode'),
        run: () => setThemeSettings({ mode: useThemeStore.getState().settings.mode === 'dark' ? 'light' : 'dark' }),
      },
      {
        id: 'toggle-locale',
        title: t('titlebar.theme'),
        run: () => setLocale(locale === 'en' ? 'zh-CN' : 'en'),
      },
    ],
    [t, locale, setLocale],
  )

  return (
    <ThemeProvider>
      <ErrorBoundary>
        <div className={styles.appRoot}>
          <TitleBar branch="main" onOpenPalette={() => setPaletteOpen(true)} />
          <div className={styles.appBody}>
            <AppShell
              sessions={<div className={styles.zonePad} data-testid="zone-sessions">{t('zone.sessions')}</div>}
              conversation={<div className={styles.zonePad} data-testid="zone-conversation">{t('zone.conversation')}</div>}
              workspace={<div className={styles.zonePad} data-testid="zone-workspace">{t('zone.workspace')}</div>}
            />
          </div>
        </div>
        <CommandPalette open={paletteOpen} commands={commands} onClose={() => setPaletteOpen(false)} />
        <Toasts />
      </ErrorBoundary>
    </ThemeProvider>
  )
}

export function App() {
  return (
    <LocaleProvider>
      <AppInner />
    </LocaleProvider>
  )
}
