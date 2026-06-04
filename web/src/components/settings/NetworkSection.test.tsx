import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { NetworkSection } from './NetworkSection'
import * as system from '../../lib/system'

afterEach(() => vi.restoreAllMocks())

const baseCfg: system.ConfigDTO = {
  theme: 'graphite', accent: 'indigo', density: 'comfortable', language: 'en',
  transcriptVerbosity: 'normal', model: 'deepseek-v4', reasoningEffort: 'high',
  baseUrl: 'https://api.deepseek.com', autoRoute: false, escalationModel: '',
  duetEnabled: false, sandboxEnabled: false, sandboxNetwork: false,
  autoReasoning: false, autoClarify: false,
  proxyMode: 'auto', proxyScheme: 'http', proxyUrl: '', noProxy: '',
  permissionDefault: 'ask',
  vimKeybindings: true, maxReadBytes: 5242880, maxWriteBytes: 5242880,
  sessionTTLDays: 90, sessionSnapshotKeep: 30, sessionAutoResumeAge: 24,
  transparentBackground: false,
}

describe('NetworkSection', () => {
  it('offers the four proxy modes', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg })
    render(<NetworkSection />)
    const sel = (await screen.findByLabelText(/proxy mode/i)) as HTMLSelectElement
    const values = Array.from(sel.options).map((o) => o.value)
    expect(values).toEqual(expect.arrayContaining(['auto', 'env', 'custom', 'off']))
  })

  it('reveals the URL + no_proxy fields only in custom mode', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg })
    vi.spyOn(system, 'saveConfig').mockResolvedValue({ ...baseCfg, proxyMode: 'custom' })
    render(<NetworkSection />)
    await userEvent.selectOptions(await screen.findByLabelText(/proxy mode/i), 'custom')
    await waitFor(() => {
      expect(screen.getByLabelText(/proxy url/i)).toBeInTheDocument()
      expect(screen.getByLabelText(/no_proxy/i)).toBeInTheDocument()
    })
  })

  it('shows the non-UTF-8/CJK round-trip note', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg })
    render(<NetworkSection />)
    await waitFor(() => expect(screen.getByText(/GBK|GB18030|non-UTF-8/i)).toBeInTheDocument())
  })
})
