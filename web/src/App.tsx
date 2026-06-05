import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { WorkspacePanel } from './components/WorkspacePanel'
import { SessionRail } from './components/SessionRail'
import { HistoryDrawer } from './components/HistoryDrawer'
import { StatusBarLive } from './components/StatusBarLive'
import { Cockpit } from './components/cockpit/Cockpit'
import { SettingsWindow } from './components/settings/SettingsWindow'
import { OnboardingWizard } from './components/OnboardingWizard'
import { UpdateBanner } from './components/UpdateBanner'
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
import { pushToast } from './lib/toasts'
import { setThemeSettings, useThemeStore } from './lib/theme/store'
import { useSessionStore } from './lib/sessionStore'
import { fetchOnboarding, fetchUpdate, type UpdateInfo } from './lib/system'
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
  renameSession,
} from './lib/api'
import type { PermissionRequest, PermissionDecision, AskRequest, AskAnswer, PlanItem, ToolStartEvent, ToolDeltaEvent, ToolEndEvent, RoutingEvent, TurnDoneEvent, PlanUpdateEvent } from './lib/api'
import { applyEvent } from './lib/transcript'
import type { TranscriptItem } from './lib/transcript'
import type { AutonomyMode } from './components/AutonomyToggle'
import styles from './components/shell/index.module.css'

const EMPTY_PERMISSION: PermissionRequest = { id: '', tool: '', args: {}, options: [] }

type WorkspaceTab = 'cockpit' | 'workspace'

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
  const [composerDraft, setComposerDraft] = useState<string | undefined>(undefined)

  // Wave-4 state
  const [pendingPermission, setPendingPermission] = useState<PermissionRequest | null>(null)
  const [pendingAsk, setPendingAsk] = useState<AskRequest | null>(null)
  const [planItems, setPlanItems] = useState<PlanItem[]>([])

  // Wave-3 state: sessions zone + workspace tabs
  const sessions = useSessionStore((s) => s.sessions)
  const activeId = useSessionStore((s) => s.activeId)
  const loadSessions = useSessionStore((s) => s.load)
  const createSession = useSessionStore((s) => s.create)
  const removeSession = useSessionStore((s) => s.remove)
  const setActiveSession = useSessionStore((s) => s.setActive)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [workspaceTab, setWorkspaceTab] = useState<WorkspaceTab>('cockpit')

  // Wave-6 state: settings / onboarding / update banner
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [onboardingOpen, setOnboardingOpen] = useState(false)
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null)

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

  // Load sessions once on mount so the SessionRail has data.
  useEffect(() => {
    void loadSessions()
  }, [loadSessions])

  // First-run onboarding + update check. Both fail closed/quiet: a failed fetch
  // simply leaves the overlay/banner hidden.
  useEffect(() => {
    fetchOnboarding()
      .then((s) => {
        if (s.needsOnboarding) setOnboardingOpen(true)
      })
      .catch(() => {})
    fetchUpdate()
      .then((u) => {
        if (u.updateAvailable) setUpdateInfo(u)
      })
      .catch(() => {})
  }, [])

  async function handleSubmit(payload: ComposerPayload) {
    dispatch({ kind: 'user', text: payload.text, pills: payload.pills })
    setStreaming(true)

    // The stream handlers are identical regardless of when we connect; extract
    // them so we can subscribe BEFORE the POST for an existing session.
    const handlers = {
      onMessageDelta: (text: string) => dispatch({ kind: 'message_delta', text }),
      onThinkingDelta: (text: string) => dispatch({ kind: 'thinking_delta', text }),
      onToolStart: (e: ToolStartEvent) =>
        dispatch({ kind: 'tool_start', id: e.id, name: e.name, args: e.args, read_only: e.read_only }),
      onToolDelta: (e: ToolDeltaEvent) => dispatch({ kind: 'tool_delta', id: e.id, delta: e.delta }),
      onToolEnd: (e: ToolEndEvent) =>
        dispatch({ kind: 'tool_end', id: e.id, result: e.result, is_error: e.is_error }),
      onRouting: (e: RoutingEvent) =>
        dispatch({ kind: 'routing', from: e.from, to: e.to, reason: e.reason }),
      onTurnDone: (e: TurnDoneEvent) => {
        dispatch({ kind: 'turn_done', stop_reason: e.stop_reason })
        clientRef.current.close()
        setStreaming(false)
        setWorkspaceRefreshKey((k) => k + 1)
        void loadSessions()
      },
      onPermissionRequest: (req: PermissionRequest) => setPendingPermission(req),
      onAskRequest: (req: AskRequest) => setPendingAsk(req),
      onPlanUpdate: (u: PlanUpdateEvent) => setPlanItems(u.items),
    }

    // Existing session: subscribe FIRST so no frame is missed before the POST
    // resolves. New session (id unknown until POST returns): connect after, and
    // the gateway replay buffer (Task 1) recovers any frames emitted in the gap.
    if (sessionId) {
      clientRef.current.close()
      clientRef.current.openEventStream(sessionId, handlers)
    }

    let sid: string
    try {
      sid = await submitPrompt(payload.text, sessionId ?? undefined)
    } catch (e) {
      setStreaming(false)
      clientRef.current.close()
      pushToast({ kind: 'danger', message: t('composer.sendFailed', 'Could not send message: ') + (e instanceof Error ? e.message : String(e)) })
      return
    }

    setSessionId(sid)
    setActiveSession(sid)
    void loadSessions()

    // If the id changed (new session) or we hadn't subscribed yet, (re)connect on
    // the real id; replay backfills anything already emitted.
    if (sid !== sessionId) {
      clientRef.current.close()
      clientRef.current.openEventStream(sid, handlers)
    }
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
  // Contract 4 non-negotiable: add-to-chat content must be appended to the composer draft.
  const handleAddToChat = (content: string) => {
    setComposerDraft((d) => (d ? d + '\n\n' + content : content))
  }

  // Map the flat TranscriptMessage[] returned by switchSession back to the
  // typed TranscriptItem[] the Transcript component renders.
  function transcriptFromMessages(msgs: TranscriptMessage[]): TranscriptItem[] {
    return msgs.map((m): TranscriptItem => {
      if (m.role === 'user') return { type: 'user', text: m.text }
      return { type: 'assistant', text: m.text, streaming: false }
    })
  }

  // Wave-3: selecting a session sets the active id and repaints the transcript
  // from its persisted history (reusing the Wave-5 switchSession path).
  const onSelectSession = useCallback(async (id: string) => {
    setSessionId(id)
    setActiveSession(id)
    setHistoryOpen(false)
    const res = await switchSession(id)
    setItems(transcriptFromMessages(res.messages))
    setWorkspaceRefreshKey((k) => k + 1)
  }, [setActiveSession])

  const onNewSession = useCallback(async () => {
    // create() returns the new session; use its id directly (never reach back into
    // the store, which would read a stale activeId if create() failed).
    try {
      const s = await createSession('')
      setSessionId(s.id)
      setItems([])
      setPlanItems([])
    } catch {
      // create failed; sessionStore records the error. Leave turn state unchanged.
    }
  }, [createSession])

  const onDeleteSession = useCallback(
    async (id: string) => {
      await removeSession(id)
      if (id === sessionId) {
        setSessionId(null)
        setItems([])
        setPlanItems([])
      }
    },
    [removeSession, sessionId],
  )

  const onRenameSession = useCallback(
    async (id: string, title: string) => {
      await renameSession(id, title)
      await loadSessions()
    },
    [loadSessions],
  )

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
    setActiveSession(child.session_id)
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
      { id: 'new-session', title: t('app.newSession'), run: () => void onNewSession() },
      {
        id: 'open-settings',
        title: t('settings.title', 'Settings'),
        run: () => setSettingsOpen(true),
      },
      {
        id: 'toggle-mode',
        title: t('titlebar.toggleMode'),
        run: () => setThemeSettings({ mode: useThemeStore.getState().settings.mode === 'dark' ? 'light' : 'dark' }),
      },
      {
        id: 'toggle-locale',
        title: t('titlebar.toggleLocale', 'Toggle language'),
        run: () => setLocale(locale === 'en' ? 'zh-CN' : 'en'),
      },
    ],
    [t, locale, setLocale, onNewSession],
  )

  const sessionsZone = (
    <div className={styles.sessionsZone} data-testid="zone-sessions">
      <SessionRail
        sessions={sessions}
        activeId={activeId}
        onNew={() => void onNewSession()}
        onSelect={(id) => void onSelectSession(id)}
        onDelete={(id) => void onDeleteSession(id)}
        onRename={(id, title) => void onRenameSession(id, title)}
        onOpenHistory={() => setHistoryOpen(true)}
      />
      <HistoryDrawer
        open={historyOpen}
        sessions={sessions}
        onResume={(id) => void onSelectSession(id)}
        onDelete={(id) => void onDeleteSession(id)}
        onRename={(id, title) => void onRenameSession(id, title)}
        onClose={() => setHistoryOpen(false)}
      />
    </div>
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
        draft={composerDraft}
      />
      <PermissionModal
        open={pendingPermission !== null}
        request={pendingPermission ?? EMPTY_PERMISSION}
        onDecide={onPermissionDecision}
      />
    </div>
  )

  const workspaceZone = (
    <div data-testid="zone-workspace" className={styles.workspaceZone}>
      <div className={styles.workspaceTabs} role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={workspaceTab === 'cockpit'}
          data-testid="ws-tab-cockpit"
          className={styles.workspaceTab}
          onClick={() => setWorkspaceTab('cockpit')}
        >
          {t('workspace.cockpitTab', 'Cockpit')}
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={workspaceTab === 'workspace'}
          data-testid="ws-tab-workspace"
          className={styles.workspaceTab}
          onClick={() => setWorkspaceTab('workspace')}
        >
          {t('workspace.workspaceTab', 'Workspace')}
        </button>
      </div>
      {workspaceTab === 'cockpit' ? (
        <Cockpit sessionId={sessionId ?? undefined} />
      ) : (
        <WorkspacePanel refreshKey={workspaceRefreshKey} onAddToChat={handleAddToChat} />
      )}
    </div>
  )

  return (
    <ThemeProvider>
      <ErrorBoundary>
        <div className={styles.appRoot}>
          {updateInfo && <UpdateBanner info={updateInfo} onDismiss={() => setUpdateInfo(null)} />}
          <TitleBar branch="main" onOpenPalette={() => setPaletteOpen(true)} onOpenSettings={() => setSettingsOpen(true)} />
          <div className={styles.appBody}>
            <AppShell sessions={sessionsZone} conversation={conversation} workspace={workspaceZone} />
          </div>
          <div data-testid="hero-statusbar">
            <StatusBarLive sessionId={sessionId ?? undefined} status={streaming ? 'streaming' : 'idle'} />
          </div>
        </div>
        <CommandPalette open={paletteOpen} commands={commands} onClose={() => setPaletteOpen(false)} />
        <SettingsWindow open={settingsOpen} onClose={() => setSettingsOpen(false)} />
        <OnboardingWizard open={onboardingOpen} onComplete={() => setOnboardingOpen(false)} />
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
