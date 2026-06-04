import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { AppearanceSection } from './AppearanceSection'
import * as system from '../../lib/system'
import * as theme from '../../lib/theme/store'

afterEach(() => vi.restoreAllMocks())

const baseCfg: system.ConfigDTO = {
  theme: 'graphite', accent: 'indigo', density: 'comfortable', language: 'en',
  transcriptVerbosity: 'normal', model: 'deepseek-v4', reasoningEffort: 'high',
  baseUrl: '', autoRoute: false, escalationModel: '', duetEnabled: false,
  sandboxEnabled: false, sandboxNetwork: false, autoReasoning: false, autoClarify: false,
  proxyMode: 'auto', proxyScheme: 'http', proxyUrl: '', noProxy: '',
}

describe('AppearanceSection', () => {
  it('applies the theme live (setThemeSettings) and persists on change', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg })
    const save = vi.spyOn(system, 'saveConfig').mockResolvedValue({ ...baseCfg, theme: 'lumen' })
    const apply = vi.spyOn(theme, 'setThemeSettings').mockImplementation(() => {})
    render(<AppearanceSection />)
    const sel = (await screen.findByLabelText(/theme/i)) as HTMLSelectElement
    await userEvent.selectOptions(sel, 'lumen')
    await waitFor(() => {
      expect(apply).toHaveBeenCalledWith(expect.objectContaining({ theme: 'lumen' }))
      expect(save).toHaveBeenCalledWith(expect.objectContaining({ theme: 'lumen' }))
    })
  })
})
