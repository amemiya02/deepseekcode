import { useEffect, useMemo, useReducer, useRef, useState } from 'react'
import { LocaleProvider, useLocale, useT } from './lib/i18n'
import { ThemeProvider } from './components/shell/ThemeProvider'
import { ErrorBoundary } from './components/shell/ErrorBoundary'
import { AppShell } from './components/shell/AppShell'
import { TitleBar } from './components/shell/TitleBar'
import { CommandPalette, type Command } from './components/shell/CommandPalette'
import { Toasts } from './components/shell/Toasts'
import { isCmdK } from './lib/shortcuts'
import { setThemeSettings, useThemeStore } from './lib/theme/store'
import { Transcript } from './components/Transcript'
import { Composer, type ComposerPayload } from './components/Composer'
import { PermissionModal } from './components/PermissionModal'
import { AskCard } from './components/AskCard'
import { PlanTodoPanel } from './components/PlanTodoPanel'
import {
  GatewayClient,
  submitPrompt,
  respondPermission,
  respondAnswer,
  cancelTurn,
} from './lib/api'
import type { PermissionRequest, PermissionDecision, AskRequest, AskAnswer, PlanItem } from './lib/api'
import { applyEvent } from './lib/transcript'
import type { TranscriptItem } from './lib/transcript'
import type { AutonomyMode } from './components/AutonomyToggle'
import styles from './components/shell/index.module.css'

const EMPTY_PERMISSION: PermissionRequest = { id: '', tool: '', args: {}, options: [] }

// AppInner lives under LocaleProvider so hooks (useT/useLocale) have their context.
function AppInner() {
  const t = useT()
  const { locale, setLocale } = useLocale()
  const [paletteOpen, setPaletteOpen] = useState(false)

  // Turn state
  const [sessionId, setSessionId] = useState<string | null>(null)
  const [streaming, setStreaming] = useState(false)
  const [mode, setMode] = useState<AutonomyMode>('ask')
  const [items, dispatch] = useReducer(
    (state: TranscriptItem[], event: Parameters<typeof applyEvent>[1]) => applyEvent(state, event),
    [],
  )

  // Wave-4 state
  const [pendingPermission, setPendingPermission] = useState<PermissionRequest | null>(null)
  const [pendingAsk, setPendingAsk] = useState<AskRequest | null>(null)
  const [planItems, setPlanItems] = useState<PlanItem[]>([])

  const clientRef = useRef(new GatewayClient())

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

  async function handleSubmit(payload: ComposerPayload) {
    const sid = await submitPrompt(payload.text, sessionId ?? undefined)
    setSessionId(sid)
    setStreaming(true)
    dispatch({ kind: 'user', text: payload.text, pills: payload.pills })
    clientRef.current.openEventStream(sid, {
      onMessageDelta: (text) => dispatch({ kind: 'message_delta', text }),
      onThinkingDelta: (text) => dispatch({ kind: 'thinking_delta', text }),
      onToolStart: (e) => dispatch({ kind: 'tool_start', id: e.id, name: e.name, args: e.args, read_only: e.read_only }),
      onToolDelta: (e) => dispatch({ kind: 'tool_delta', id: e.id, delta: e.delta }),
      onToolEnd: (e) => dispatch({ kind: 'tool_end', id: e.id, result: e.result, is_error: e.is_error }),
      onRouting: (e) => dispatch({ kind: 'routing', from: e.from, to: e.to, reason: e.reason }),
      onTurnDone: (e) => {
        dispatch({ kind: 'turn_done', stop_reason: e.stop_reason })
        setStreaming(false)
      },
      // Wave-4 handlers
      onPermissionRequest: (req) => setPendingPermission(req),
      onAskRequest: (req) => setPendingAsk(req),
      onPlanUpdate: (u) => setPlanItems(u.items),
    })
  }

  async function onPermissionDecision(d: PermissionDecision) {
    const req = pendingPermission
    setPendingPermission(null)
    if (req) await respondPermission(req.id, d)
  }

  async function onAskAnswer(a: AskAnswer) {
    setPendingAsk(null)
    await respondAnswer(a)
  }

  async function onAskDismiss() {
    setPendingAsk(null)
  }

  async function onStop() {
    try {
      if (sessionId) await cancelTurn(sessionId)
    } finally {
      setStreaming(false)
    }
  }

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

  const conversation = (
    <div data-testid="zone-conversation" className="conversation-zone">
      <Transcript items={items} />
      <PlanTodoPanel items={planItems} onDismiss={() => setPlanItems([])} />
      {pendingAsk && <AskCard request={pendingAsk} onAnswer={onAskAnswer} onDismiss={onAskDismiss} />}
      <Composer
        streaming={streaming}
        mode={mode}
        commands={[]}
        onSend={handleSubmit}
        onCancel={onStop}
        onModeChange={setMode}
      />
      <PermissionModal
        open={pendingPermission !== null}
        request={pendingPermission ?? EMPTY_PERMISSION}
        onDecide={onPermissionDecision}
      />
    </div>
  )

  return (
    <ThemeProvider>
      <ErrorBoundary>
        <div className={styles.appRoot}>
          <TitleBar branch="main" onOpenPalette={() => setPaletteOpen(true)} />
          <div className={styles.appBody}>
            <AppShell
              sessions={<div className={styles.zonePad} data-testid="zone-sessions">{t('zone.sessions')}</div>}
              conversation={conversation}
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
