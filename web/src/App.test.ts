// Tests for App.svelte handleSubmit / streaming path.
// GatewayClient is mocked so no real EventSource is opened.
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import App from './App.svelte'
import * as api from './lib/api'

// ---- helpers ----------------------------------------------------------------

type Handlers = {
  onDelta?: (text: string) => void
  onTool?: (name: string) => void
  onDone?: () => void
  onError?: (e: Event) => void
}

function makeClientMock() {
  let captured: Handlers = {}
  const openEventStream = vi.fn((_id: string, h: Handlers) => { captured = h })
  const close = vi.fn()
  return { openEventStream, close, captured: () => captured }
}

// ---- tests ------------------------------------------------------------------

describe('App handleSubmit streaming', () => {
  beforeEach(() => {
    // submitPrompt resolves with a stable request id.
    vi.spyOn(api, 'submitPrompt').mockResolvedValue('req-test')
  })

  it('appends a single assistant bubble that grows with each onDelta call', async () => {
    const mock = makeClientMock()
    vi.spyOn(api, 'GatewayClient' as keyof typeof api).mockImplementation(
      () => mock as unknown as api.GatewayClient,
    )

    render(App)

    const input = screen.getByPlaceholderText('Type a prompt…')
    const sendBtn = screen.getByText('Send')

    // Submit a prompt.
    await fireEvent.input(input, { target: { value: 'hello' } })
    await fireEvent.click(sendBtn)

    // Wait for submitPrompt to resolve and openEventStream to be called.
    await waitFor(() => expect(mock.openEventStream).toHaveBeenCalledOnce())

    const handlers = mock.captured()

    // First delta.
    handlers.onDelta?.('Hel')
    await waitFor(() => expect(screen.getByText('Hel')).toBeTruthy())

    // Second delta — text accumulates; still exactly ONE assistant bubble.
    handlers.onDelta?.('lo!')
    await waitFor(() => {
      expect(screen.getByText('Hello!')).toBeTruthy()
      // 'Hel' node should no longer exist as a standalone text.
      expect(screen.queryAllByText('Hel')).toHaveLength(0)
    })

    // onDone clears streaming state — input must become enabled again.
    handlers.onDone?.()
    await waitFor(() =>
      expect((screen.getByPlaceholderText('Type a prompt…') as HTMLInputElement).disabled).toBe(false),
    )
  })

  it('does not duplicate the assistant bubble on an empty-string delta', async () => {
    const mock = makeClientMock()
    vi.spyOn(api, 'GatewayClient' as keyof typeof api).mockImplementation(
      () => mock as unknown as api.GatewayClient,
    )

    render(App)

    const input = screen.getByPlaceholderText('Type a prompt…')
    await fireEvent.input(input, { target: { value: 'ping' } })
    await fireEvent.click(screen.getByText('Send'))
    await waitFor(() => expect(mock.openEventStream).toHaveBeenCalledOnce())

    const handlers = mock.captured()

    // Real SSE may send empty-string deltas; this must not create a second bubble.
    handlers.onDelta?.('')
    handlers.onDelta?.('pong')
    await waitFor(() => {
      const bubbles = screen.queryAllByText('pong')
      expect(bubbles).toHaveLength(1)
    })
  })

  it('calls client.close() when onError fires', async () => {
    const mock = makeClientMock()
    vi.spyOn(api, 'GatewayClient' as keyof typeof api).mockImplementation(
      () => mock as unknown as api.GatewayClient,
    )

    render(App)

    const input = screen.getByPlaceholderText('Type a prompt…')
    await fireEvent.input(input, { target: { value: 'x' } })
    await fireEvent.click(screen.getByText('Send'))
    await waitFor(() => expect(mock.openEventStream).toHaveBeenCalledOnce())

    mock.captured().onError?.(new Event('error'))
    await waitFor(() => expect(mock.close).toHaveBeenCalled())
    // Input must be re-enabled after error.
    expect((screen.getByPlaceholderText('Type a prompt…') as HTMLInputElement).disabled).toBe(false)
  })
})
