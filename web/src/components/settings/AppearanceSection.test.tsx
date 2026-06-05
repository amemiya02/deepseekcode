import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { AppearanceSection } from './AppearanceSection'
import * as system from '../../lib/system'
import * as store from '../../lib/theme/store'

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

describe('AppearanceSection', () => {
  it('applies accent live (setThemeSettings) and persists on change', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg })
    const save = vi.spyOn(system, 'saveConfig').mockResolvedValue({ ...baseCfg, accent: 'rose' })
    const apply = vi.spyOn(store, 'setThemeSettings').mockImplementation(() => {})
    render(<AppearanceSection />)
    const sel = (await screen.findByLabelText(/accent/i)) as HTMLSelectElement
    await userEvent.selectOptions(sel, 'rose')
    await waitFor(() => {
      expect(apply).toHaveBeenCalledWith(expect.objectContaining({ accent: 'rose' }))
      expect(save).toHaveBeenCalledWith(expect.objectContaining({ accent: 'rose' }))
    })
  })

  describe('patch does not wipe theme or density', () => {
    beforeEach(() => {
      vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg })
      vi.spyOn(system, 'saveConfig').mockResolvedValue({ ...baseCfg, accent: 'rose' })
    })

    it('changing accent does not wipe theme or density', async () => {
      const apply = vi.spyOn(store, 'setThemeSettings').mockImplementation(() => {})
      render(<AppearanceSection />)
      const accent = await screen.findByDisplayValue('indigo')
      fireEvent.change(accent, { target: { value: 'rose' } })
      await waitFor(() => {
        expect(apply).toHaveBeenCalledWith({ accent: 'rose' })
      })
      // Must NOT have been called with an undefined theme.
      expect(apply).not.toHaveBeenCalledWith(
        expect.objectContaining({ theme: undefined }),
      )
    })
  })
})
