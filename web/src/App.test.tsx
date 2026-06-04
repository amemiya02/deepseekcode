import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { App } from './App'
import { useThemeStore, DEFAULT_THEME_SETTINGS } from './lib/theme/store'

beforeEach(() => {
  localStorage.clear()
  useThemeStore.setState({ settings: { ...DEFAULT_THEME_SETTINGS } })
})

describe('App shell composition', () => {
  it('renders the three workspace zones', () => {
    render(<App />)
    expect(screen.getByTestId('zone-sessions')).toBeInTheDocument()
    expect(screen.getByTestId('zone-conversation')).toBeInTheDocument()
    expect(screen.getByTestId('zone-workspace')).toBeInTheDocument()
  })

  it('applies theme tokens to :root via ThemeProvider', () => {
    render(<App />)
    expect(document.documentElement.style.getPropertyValue('--bg')).toContain('oklch')
  })

  it('opens the command palette when the titlebar trigger is clicked', async () => {
    const user = userEvent.setup()
    render(<App />)
    expect(screen.queryByPlaceholderText(/Search commands/)).toBeNull()
    await user.click(screen.getByTestId('open-palette'))
    expect(screen.getByPlaceholderText(/Search commands/)).toBeInTheDocument()
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
    respondPermission: vi.fn().mockResolvedValue(undefined),
    respondAnswer: vi.fn().mockResolvedValue(undefined),
    cancelTurn: vi.fn().mockResolvedValue(undefined),
    GatewayClient: class {
      openEventStream(_sid: string, handlers: any) { captured = handlers }
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
})

describe('App workspace zone (Wave 5)', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    // Route fetch calls: files listing → entries; add-to-chat → content pill.
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (typeof url === 'string' && url.includes('add-to-chat')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: async () => ({ content: 'main.go' }),
          } as Response)
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: async () => ({ entries: [{ name: 'main.go', path: 'main.go', is_dir: false }] }),
        } as Response)
      }),
    )
    // EventSource is referenced by the App's stream client; stub a no-op.
    vi.stubGlobal(
      'EventSource',
      vi.fn().mockImplementation(() => ({ close: vi.fn(), addEventListener: vi.fn() })),
    )
  })

  it('mounts WorkspacePanel into the workspace zone', async () => {
    render(<App />)
    const zone = screen.getByTestId('zone-workspace')
    await waitFor(() => expect(zone.querySelector('[data-testid="tab-files"]')).not.toBeNull())
  })

  it('add-to-chat appends content to the composer draft (Contract 4)', async () => {
    render(<App />)
    // WorkspacePanel exposes an add-to-chat button per file; simulate it by
    // finding the first such button after the panel mounts.
    const zone = screen.getByTestId('zone-workspace')
    await waitFor(() => expect(zone.querySelector('[data-testid="tab-files"]')).not.toBeNull())
    // Locate the add-to-chat button rendered by FileTree for the mock file.
    // FileTree renders data-testid={`add-${entry.name}`}.
    const addBtn = await screen.findByTestId('add-main.go')
    fireEvent.click(addBtn)
    // The composer input should now contain the appended content.
    const input = screen.getByTestId('composer-input') as HTMLTextAreaElement
    await waitFor(() => expect(input.value).toContain('main.go'))
  })
})
