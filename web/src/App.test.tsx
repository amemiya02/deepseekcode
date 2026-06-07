import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { App } from './App'
import { useThemeStore, DEFAULT_THEME_SETTINGS } from './lib/theme/store'
import { useLayoutStore } from './lib/layoutStore'
import * as system from './lib/system'
import * as api from './lib/api'

beforeEach(() => {
  localStorage.clear()
  useThemeStore.setState({ settings: { ...DEFAULT_THEME_SETTINGS } })
})

describe('App shell composition', () => {
  it('renders the three workspace zones when review pane is open', async () => {
    // Default reviewPin is 'closed' — workspace hidden. Open it first.
    useLayoutStore.getState().toggleRight(false)
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (typeof url === 'string' && url.includes('/v1/changed')) {
          return Promise.resolve({
            ok: true, status: 200,
            json: async () => ({ entries: [{ path: 'main.go', status: 'M', deleted: false }] }),
          } as Response)
        }
        return Promise.resolve({ ok: true, status: 200, json: async () => ({ entries: [] }) } as Response)
      }),
    )
    vi.stubGlobal('EventSource', vi.fn().mockImplementation(() => ({ close: vi.fn(), addEventListener: vi.fn() })))
    render(<App />)
    expect(screen.getByTestId('zone-sessions')).toBeInTheDocument()
    expect(screen.getByTestId('zone-conversation')).toBeInTheDocument()
    expect(await screen.findByTestId('zone-workspace')).toBeInTheDocument()
  })

  it('applies theme tokens to :root via ThemeProvider', () => {
    render(<App />)
    // Brand light default (spec §3.2): --bg is the exact brand hex, not OKLCH.
    expect(document.documentElement.style.getPropertyValue('--bg')).toBe('#f8f9fb')
  })

  it('no longer renders the titlebar command-palette pill', () => {
    render(<App />)
    expect(screen.queryByTestId('open-palette')).toBeNull()
  })

  it('opens the command palette on Cmd+K', () => {
    render(<App />)
    fireEvent.keyDown(window, { key: 'k', metaKey: true })
    expect(screen.getByPlaceholderText(/Search commands/)).toBeInTheDocument()
  })
})

// Capture the StreamHandlers App registers so the test can drive SSE events,
// and stub the REST verbs so we can assert what App posts.
let captured: any = null
vi.mock('./lib/api', async (orig) => {
  const actual = await (orig() as Promise<any>)
  return {
    ...actual,
    submitPrompt: vi.fn().mockResolvedValue('sess-1'),
    steerTurn: vi.fn().mockResolvedValue(undefined),
    respondPermission: vi.fn().mockResolvedValue(undefined),
    respondAnswer: vi.fn().mockResolvedValue(undefined),
    cancelTurn: vi.fn().mockResolvedValue(undefined),
    // fetchBalance is invoked by the cockpit/hero status bar on connect; resolve
    // it so those live subscriptions don't fire a real (relative-URL) fetch.
    fetchBalance: vi.fn().mockResolvedValue({ provider: 'deepseek', currency: 'CNY', amount: 0 }),
    GatewayClient: class {
      openEventStream(_sid: string, handlers: any) {
        // Both App's turn stream and the cockpit store's live stream share this
        // mocked client. Only capture App's TURN handler set (it carries the
        // permission/ask/plan callbacks) so cockpit's connect doesn't clobber it.
        if (handlers && handlers.onPermissionRequest) captured = handlers
      }
      close() {}
    },
  }
})

// startTurn submits a prompt through the composer so a turn is streaming and the
// captured handlers are live. The composer's testid comes from Wave 2's Composer.
// Note: plan spec used 'send-stop-btn' but the real testid from SendStopButton is 'send-stop'.
async function startTurn(user: ReturnType<typeof userEvent.setup>) {
  const input = screen.getByTestId('composer-input')
  await user.type(input, 'go')
  await user.click(screen.getByTestId('send-stop'))
}

describe('App — Wave 4 wiring', () => {
  beforeEach(() => {
    captured = null
    vi.clearAllMocks()
  })

  it('shows a PermissionModal on permission_request and routes the decision', async () => {
    const api = await import('./lib/api')
    const user = userEvent.setup()
    render(<App />)
    await startTurn(user)
    captured.onPermissionRequest({ id: 'perm-1', tool: 'bash', args: { command: 'ls' }, options: [] })
    await user.click(await screen.findByTestId('perm-once'))
    expect(api.respondPermission).toHaveBeenCalledWith('perm-1', 'once')
  })

  it('shows an AskCard on ask_request and routes the answer', async () => {
    const api = await import('./lib/api')
    const user = userEvent.setup()
    render(<App />)
    await startTurn(user)
    captured.onAskRequest({
      id: 'ask-1',
      questions: [{ question: 'Q?', header: 'h', multiple: false, options: [{ label: 'Yes', description: '' }] }],
    })
    await user.click(await screen.findByText('Yes'))
    expect(api.respondAnswer).toHaveBeenCalledWith({ id: 'ask-1', questionIndex: 0, choices: ['Yes'] })
  })

  it('renders the plan panel on plan_update', async () => {
    const user = userEvent.setup()
    render(<App />)
    await startTurn(user)
    captured.onPlanUpdate({ items: [{ text: 'step a', status: 'in_progress' }] })
    expect(await screen.findByText('step a')).toBeInTheDocument()
  })

  it('Stop button calls cancelTurn while streaming', async () => {
    const api = await import('./lib/api')
    const user = userEvent.setup()
    render(<App />)
    await startTurn(user)
    await user.click(await screen.findByTestId('send-stop'))
    expect(api.cancelTurn).toHaveBeenCalledWith('sess-1')
  })

  it('mid-turn steer: submitting while streaming calls steerTurn (not submitPrompt) and echoes the user bubble', async () => {
    const apiMod = await import('./lib/api')
    const user = userEvent.setup()
    render(<App />)
    // First turn — sets streaming=true, sessionId='sess-1'
    await startTurn(user)
    // Reset call counts so we can assert cleanly on the second submit
    vi.clearAllMocks()
    // Type a second message while streaming is active and submit
    const input = screen.getByTestId('composer-input')
    await user.type(input, 'redirect now{Enter}')
    expect(apiMod.steerTurn).toHaveBeenCalledWith('sess-1', 'redirect now')
    expect(apiMod.submitPrompt).not.toHaveBeenCalled()
    // The user bubble must be echoed in the transcript
    expect(await screen.findAllByText('redirect now')).not.toHaveLength(0)
  })
})

describe('App workspace zone (SP5)', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    // Default reviewPin is 'closed' — open it for workspace tests
    useLayoutStore.setState((s) => ({ layout: { ...s.layout, reviewPin: 'open' as const } }))
    // Route fetch calls: changed files → entries; diff → patch; file → content.
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (typeof url === 'string' && url.includes('/v1/changed')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: async () => ({ entries: [{ path: 'main.go', status: 'M', deleted: false }] }),
          } as Response)
        }
        if (typeof url === 'string' && url.includes('/v1/diff')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: async () => ({ path: 'main.go', patch: '' }),
          } as Response)
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({ entries: [] }),
        } as Response)
      }),
    )
    // EventSource is referenced by the App's stream client; stub a no-op.
    vi.stubGlobal(
      'EventSource',
      vi.fn().mockImplementation(() => ({ close: vi.fn(), addEventListener: vi.fn() })),
    )
  })

  it('mounts review-hero and telemetry-toggle inside zone-workspace (no tabs)', async () => {
    render(<App />)
    // Wait for hasChanges to resolve (fetchChanged effect is async)
    const zone = await screen.findByTestId('zone-workspace')
    // No workspace tabs — the old cockpit/workspace tablist is gone
    expect(within(zone).queryByTestId('ws-tab-cockpit')).toBeNull()
    expect(within(zone).queryByTestId('ws-tab-workspace')).toBeNull()
    // Review hero and telemetry toggle are rendered directly
    expect(await within(zone).findByTestId('review-hero')).toBeInTheDocument()
    expect(within(zone).getByTestId('telemetry-toggle')).toBeInTheDocument()
  })
})

// Wave 0 final integration: SessionRail, hero StatusBar, Cockpit tab, Settings,
// Onboarding, UpdateBanner are all composed into the shell. These tests mock the
// system/update/onboarding reads and the sessions listing so rendering is
// deterministic, then assert each surface mounts.
describe('App — full shell integration (Wave 0)', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    // Default reviewPin is 'closed' — open it for workspace tests
    useLayoutStore.setState((s) => ({ layout: { ...s.layout, reviewPin: 'open' as const } }))
    // Fail-closed by default: no onboarding, no update — keeps overlays hidden
    // unless a test opts in.
    vi.spyOn(system, 'fetchOnboarding').mockResolvedValue({
      needsOnboarding: false,
      baseUrl: '',
      model: '',
    })
    vi.spyOn(system, 'fetchUpdate').mockResolvedValue({
      current: '1.0.0',
      latest: '1.0.0',
      updateAvailable: false,
      url: '',
    })
    // listSessions backs the SessionRail; route fetch deterministically.
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (typeof url === 'string' && url.includes('/v1/sessions')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: async () => ({ sessions: [] }),
          } as Response)
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({ entries: [] }),
        } as Response)
      }),
    )
    vi.stubGlobal(
      'EventSource',
      vi.fn().mockImplementation(() => ({ close: vi.fn(), addEventListener: vi.fn() })),
    )
  })

  it('mounts SessionRail (with the new-session control) into the sessions zone', async () => {
    render(<App />)
    // findBy* settles the mount-time async effects (loadSessions, onboarding,
    // update) inside act so state updates don't warn.
    const zone = screen.getByTestId('zone-sessions')
    expect(await within(zone).findByTestId('session-new')).toBeInTheDocument()
    // HistoryDrawer has been deleted; history is now integrated into the rail.
    expect(within(zone).queryByTestId('session-history')).toBeNull()
  })

  it('mounts the hero status bar persistently', async () => {
    render(<App />)
    const hero = screen.getByTestId('hero-statusbar')
    // The StatusBar exposes its busy dot and live cost chip.
    expect(await within(hero).findByTestId('status-dot')).toBeInTheDocument()
    expect(within(hero).getByTestId('turn-cost')).toBeInTheDocument()
  })

  it('workspace zone has review-hero and telemetry-toggle (no tabs)', async () => {
    // Override fetch for this test so /v1/changed returns entries → hasChanges=true
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (typeof url === 'string' && url.includes('/v1/sessions')) {
          return Promise.resolve({ ok: true, status: 200, json: async () => ({ sessions: [] }) } as Response)
        }
        if (typeof url === 'string' && url.includes('/v1/changed')) {
          return Promise.resolve({
            ok: true, status: 200,
            json: async () => ({ entries: [{ path: 'main.go', status: 'M', deleted: false }] }),
          } as Response)
        }
        return Promise.resolve({ ok: true, status: 200, json: async () => ({ entries: [] }) } as Response)
      }),
    )
    render(<App />)
    // Wait for hasChanges to resolve (fetchChanged effect is async)
    const zone = await screen.findByTestId('zone-workspace')
    expect(await within(zone).findByTestId('review-hero')).toBeInTheDocument()
    expect(within(zone).getByTestId('telemetry-toggle')).toBeInTheDocument()
    expect(within(zone).queryByTestId('ws-tab-cockpit')).toBeNull()
    expect(within(zone).queryByTestId('ws-tab-workspace')).toBeNull()
  })

  it('opens the SettingsView via the titlebar gear button (full-page, no dialog)', async () => {
    const user = userEvent.setup()
    render(<App />)
    expect(screen.queryByTestId('settings-back')).toBeNull()
    await user.click(screen.getByTestId('open-settings'))
    expect(await screen.findByTestId('settings-back')).toBeInTheDocument()
    expect(screen.queryByRole('dialog', { name: 'Settings' })).toBeNull()
  })

  it('opens the SettingsView via the command palette (Cmd+K) Settings command', async () => {
    const user = userEvent.setup()
    render(<App />)
    expect(screen.queryByTestId('settings-back')).toBeNull()
    fireEvent.keyDown(window, { key: 'k', metaKey: true })
    await user.click(await screen.findByText('Settings'))
    expect(await screen.findByTestId('settings-back')).toBeInTheDocument()
    expect(screen.queryByRole('dialog', { name: 'Settings' })).toBeNull()
  })

  it('shows the OnboardingWizard when onboarding is needed', async () => {
    vi.spyOn(system, 'fetchOnboarding').mockResolvedValue({
      needsOnboarding: true,
      baseUrl: 'https://api.deepseek.com',
      model: 'deepseek-v4',
    })
    render(<App />)
    expect(await screen.findByRole('dialog', { name: 'Welcome' })).toBeInTheDocument()
  })

  it('renders the UpdateBanner when an update is available', async () => {
    vi.spyOn(system, 'fetchUpdate').mockResolvedValue({
      current: '1.0.0',
      latest: '1.2.0',
      updateAvailable: true,
      url: 'https://example.com/download',
    })
    render(<App />)
    expect(await screen.findByText('1.2.0')).toBeInTheDocument()
  })

  it('calls renameSession PATCH when onRename fires from SessionRail (App wiring)', async () => {
    const renameSpy = vi.spyOn(api, 'renameSession').mockResolvedValue(undefined)
    const user = userEvent.setup()

    // Seed one session so the rename pencil is reachable.
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (typeof url === 'string' && url.includes('/v1/sessions')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: async () => ({
              sessions: [{ id: 'rename-sess', title: 'Original title', turns: 2, created_at: 0, updated_at: 0 }],
            }),
          } as Response)
        }
        // Fallback: return entries so ChangedFiles (now always mounted in workspace zone) doesn't crash.
        return Promise.resolve({ ok: true, status: 200, json: async () => ({ entries: [] }) } as Response)
      }),
    )

    render(<App />)

    // Wait for the session item to appear in the rail.
    const zone = screen.getByTestId('zone-sessions')
    expect(await within(zone).findByText('Original title')).toBeInTheDocument()

    // Click the rename pencil to enter rename mode.
    await user.click(within(zone).getByTestId('session-rename'))

    // Clear the input and type a new title, then confirm with Enter.
    const input = within(zone).getByTestId('session-rename-input')
    await user.clear(input)
    await user.type(input, 'Renamed title{Enter}')

    // The App wiring must have called renameSession (the PATCH endpoint).
    await waitFor(() =>
      expect(renameSpy).toHaveBeenCalledWith('rename-sess', 'Renamed title'),
    )

    renameSpy.mockRestore()
  })
})

// ── Task 3 (P3.4): capability-driven effort levels ───────────────────────────
// These tests verify that App derives effortLevels from the active model's
// effort descriptor and passes them down to the Composer (which forwards them
// to EffortSwitcher — which returns null when levels.length === 0).
describe('App — P3.4 capability-driven effort (Task 3)', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.stubGlobal(
      'EventSource',
      vi.fn().mockImplementation(() => ({ close: vi.fn(), addEventListener: vi.fn() })),
    )
  })

  it('Composer shows effort trigger when active model has effort.kind==="levels" with max', async () => {
    vi.spyOn(api, 'fetchModelState').mockResolvedValue({
      active: 'deepseek-v4-flash',
      effort: 'max',
      models: [
        {
          id: 'deepseek-v4-flash',
          label: 'DeepSeek V4 Flash',
          provider: 'deepseek',
          caps: { vision: false, tools: true, reasoning: true },
          effort: { kind: 'levels', levels: ['low', 'medium', 'high', 'max'], default: 'max' },
          context: 1_000_000,
        },
      ],
    })
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ entries: [] }) } as Response),
    )
    render(<App />)
    // Wait for fetchModelState to resolve and re-render
    expect(await screen.findByTestId('effort-trigger')).toBeInTheDocument()
  })

  it('Composer hides effort trigger when active model has effort.kind==="none"', async () => {
    vi.spyOn(api, 'fetchModelState').mockResolvedValue({
      active: 'no-effort-model',
      effort: 'medium',
      models: [
        {
          id: 'no-effort-model',
          label: 'No Effort Model',
          provider: 'deepseek',
          caps: { vision: false, tools: true, reasoning: false },
          effort: { kind: 'none' },
          context: 128_000,
        },
      ],
    })
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ entries: [] }) } as Response),
    )
    render(<App />)
    // Give async effects time to settle
    await waitFor(() => {
      expect(screen.queryByTestId('effort-trigger')).toBeNull()
    })
  })

  it('effort resets to model default on switch when current effort not in new model levels', async () => {
    const flashModel = {
      id: 'deepseek-v4-flash',
      label: 'DeepSeek V4 Flash',
      provider: 'deepseek',
      caps: { vision: false, tools: true, reasoning: true },
      effort: { kind: 'levels' as const, levels: ['low', 'medium', 'high', 'max'], default: 'max' },
      context: 1_000_000,
    }
    // limitedModel has only low/medium — 'max' is not in its levels, so
    // switching to it must reset effort to its default ('low').
    const limitedModel = {
      id: 'limited-model',
      label: 'Limited Model',
      provider: 'deepseek',
      caps: { vision: false, tools: true, reasoning: false },
      effort: { kind: 'levels' as const, levels: ['low', 'medium'], default: 'low' },
      context: 64_000,
    }
    vi.spyOn(api, 'fetchModelState').mockResolvedValue({
      active: 'deepseek-v4-flash',
      effort: 'max',
      models: [flashModel, limitedModel],
    })
    // ModelSwitcher self-fetches on open — return both models so the row is clickable.
    vi.spyOn(api, 'fetchModels').mockResolvedValue([flashModel, limitedModel])
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ entries: [] }) } as Response),
    )
    render(<App />)
    // Wait for initial state: flash active, effort='max'
    expect(await screen.findByTestId('effort-trigger')).toHaveTextContent('max')
    // Open the model picker and click the limited-model row.
    // This fires onModelChange('limited-model') in App, which must detect that
    // 'max' ∉ limitedModel.effort.levels and call setEffort('low').
    const user = userEvent.setup()
    await user.click(screen.getByTestId('model-trigger'))
    await user.click(await screen.findByText('Limited Model'))
    // After the switch the effort trigger must show the new model's default.
    await waitFor(() => {
      expect(screen.getByTestId('effort-trigger')).toHaveTextContent('low')
    })
  })
})
