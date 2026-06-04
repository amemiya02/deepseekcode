import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { SandboxSection } from './SandboxSection'
import * as system from '../../lib/system'

afterEach(() => vi.restoreAllMocks())

const baseCfg: system.ConfigDTO = {
  theme: 'graphite', accent: 'indigo', density: 'comfortable', language: 'en',
  transcriptVerbosity: 'normal', model: 'deepseek-v4', reasoningEffort: 'high',
  baseUrl: '', autoRoute: false, escalationModel: '', duetEnabled: false,
  sandboxEnabled: false, sandboxNetwork: false, autoReasoning: false, autoClarify: false,
  proxyMode: 'auto', proxyScheme: 'http', proxyUrl: '', noProxy: '',
  permissionDefault: 'ask',
  vimKeybindings: true, maxReadBytes: 5242880, maxWriteBytes: 5242880,
  sessionTTLDays: 90, sessionSnapshotKeep: 30, sessionAutoResumeAge: 24,
  transparentBackground: false,
}

describe('SandboxSection', () => {
  it('toggles sandbox enabled and persists', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg })
    const save = vi.spyOn(system, 'saveConfig').mockResolvedValue({ ...baseCfg, sandboxEnabled: true })
    render(<SandboxSection />)
    const toggle = await screen.findByLabelText(/enable sandbox/i)
    await userEvent.click(toggle)
    await waitFor(() => expect(save).toHaveBeenCalledWith(expect.objectContaining({ sandboxEnabled: true })))
  })

  it('disables the network-egress toggle while sandbox is off', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg, sandboxEnabled: false })
    render(<SandboxSection />)
    const net = (await screen.findByLabelText(/network egress/i)) as HTMLInputElement
    expect(net.disabled).toBe(true)
  })
})
