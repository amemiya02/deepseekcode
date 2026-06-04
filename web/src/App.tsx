import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { WorkspacePanel } from './components/WorkspacePanel'
import { rewind, fork, summarize, switchSession } from './lib/checkpoint'
import type { TranscriptMessage } from './lib/checkpoint'
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
  const [items, setItems] = useState<TranscriptItem[]>([])
  // dispatch wraps applyEvent so existing call sites need no changes.
  const dispatch = useCallback(
    (event: Parameters<typeof applyEvent>[1]) => setItems((s) => applyEvent(s, event)),
    [],
  )

  // Wave-5 state
  const [workspaceRefreshKey, setWorkspaceRefreshKey] = useState(0)

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
    // Close any previous SSE connection before opening a new one to prevent
    // stale event handlers from a prior turn dispatching into current state.
    clientRef.current.close()
    clientRef.current.openEventStream(sid, {
      onMessageDelta: (text) => dispatch({ kind: 'message_delta', text }),
      onThinkingDelta: (text) => dispatch({ kind: 'thinking_delta', text }),
      onToolStart: (e) => dispatch({ kind: 'tool_start', id: e.id, name: e.name, args: e.args, read_only: e.read_only }),
      onToolDelta: (e) => dispatch({ kind: 'tool_delta', id: e.id, delta: e.delta }),
      onToolEnd: (e) => dispatch({ kind: 'tool_end', id: e.id, result: e.result, is_error: e.is_error }),
      onRouting: (e) => dispatch({ kind: 'routing', from: e.from, to: e.to, reason: e.reason }),
      onTurnDone: (e) => {
        dispatch({ kind: 'turn_done', stop_reason: e.stop_reason })
        // Close the SSE connection once the turn is complete so connections do
        // not accumulate across turns.
        clientRef.current.close()
        setStreaming(false)
        // Bump the workspace refresh key so ChangedFiles re-fetches after a turn.
        setWorkspaceRefreshKey((k) => k + 1)
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
      // Close the browser-side SSE connection so trailing server events after
      // cancellation do not fire into stale component state.
      clientRef.current.close()
      setStreaming(false)
    }
  }

  // Wave-5: add-to-chat handler — content is appended to the composer draft.
  // Composer manages its own internal text state; this is a stub until Wave 2's
  // Composer exposes a controlled draft prop.
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const handleAddToChat = (_content: string) => {
    // TODO(wave-5): wire into Composer draft when Composer accepts a controlled value prop
  }

  // Map the flat TranscriptMessage[] returned by switchSession back to the
  // typed TranscriptItem[] the Transcript component renders.
  function transcriptFromMessages(msgs: TranscriptMessage[]): TranscriptItem[] {
    return msgs.map((m): TranscriptItem => {
      if (m.role === 'user') return { type: 'user', text: m.text }
      return { type: 'assistant', text: m.text, streaming: false }
    })
  }

  // Wave-5: per-user-message rewind/fork/summarize callbacks that drive
  // web/src/lib/checkpoint.ts then repaint via switchSession.
  const onRewind = async (keepMessages: number, scope: 'code' | 'conversation' | 'both') => {
    if (!sessionId) return
    await rewind(sessionId, keepMessages, scope)
    const res = await switchSession(sessionId)
    // Repaint transcript from truncated history (Contract 4 non-negotiable).
    setItems(transcriptFromMessages(res.messages))
    setWorkspaceRefreshKey((k) => k + 1)
  }

  const onFork = async () => {
    if (!sessionId) return
    const child = await fork(sessionId)
    const res = await switchSession(child.session_id)
    setSessionId(child.session_id)
    setItems(transcriptFromMessages(res.messages))
  }

  const onSummarize = async (mode: 'from' | 'upto', index: number) => {
    if (!sessionId) return
    await summarize(sessionId, mode, index, '')
    const res = await switchSession(sessionId)
    setItems(transcriptFromMessages(res.messages))
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
      <Transcript items={items} rewindHandlers={{ onRewind, onFork, onSummarize }} />
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
              workspace={
                <div data-testid="zone-workspace">
                  <WorkspacePanel refreshKey={workspaceRefreshKey} onAddToChat={handleAddToChat} />
                </div>
              }
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
