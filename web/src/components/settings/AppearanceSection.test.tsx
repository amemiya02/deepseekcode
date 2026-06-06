import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { LocaleProvider, setLocale } from '../../lib/i18n'
import { AppearanceSection } from './AppearanceSection'
import * as system from '../../lib/system'
import * as store from '../../lib/theme/store'

afterEach(() => vi.restoreAllMocks())

const baseCfg: system.ConfigDTO = {
  accent: 'indigo', density: 'comfortable', language: 'en',
  model: 'deepseek-v4', reasoningEffort: 'high',
  baseUrl: '', autoRoute: false, escalationModel: '', duetEnabled: false,
  sandboxEnabled: false, sandboxNetwork: false, autoReasoning: false, autoClarify: false,
  proxyMode: 'auto', proxyScheme: 'http', proxyUrl: '', noProxy: '',
  permissionDefault: 'ask',
  vimKeybindings: true, maxReadBytes: 5242880, maxWriteBytes: 5242880,
  sessionTTLDays: 90, sessionSnapshotKeep: 30, sessionAutoResumeAge: 24,
  transparentBackground: false,
}

// Helper — open a BrandedSelect trigger by testid and click an option by text.
// BrandedSelect portals its listbox to document.body, so we must wait for the
// option to appear before clicking it.
async function pickOption(testid: string, optionText: string) {
  fireEvent.click(await screen.findByTestId(testid))
  const opt = await screen.findByRole('option', { name: optionText })
  fireEvent.click(opt)
}

describe('AppearanceSection', () => {
  beforeEach(() => {
    localStorage.clear()
    setLocale('en')
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg })
  })

  it('renders language as BrandedSelect and flips locale + saves on change', async () => {
    const save = vi.spyOn(system, 'saveConfig').mockResolvedValue({ ...baseCfg, language: 'zh' })
    render(<LocaleProvider><AppearanceSection /></LocaleProvider>)
    const trigger = await screen.findByTestId('appearance-language')
    expect(trigger).toHaveAttribute('aria-haspopup', 'listbox')
    fireEvent.click(trigger)
    const opt = await screen.findByRole('option', { name: '中文' })
    fireEvent.click(opt)
    await waitFor(() => expect(save).toHaveBeenCalledWith(expect.objectContaining({ language: 'zh' })))
  })

  it('renders accent picker using BrandedSelect (not a native select)', async () => {
    render(<LocaleProvider><AppearanceSection /></LocaleProvider>)
    const trigger = await screen.findByTestId('appearance-accent')
    expect(trigger.tagName.toLowerCase()).toBe('button')
    expect(trigger).toHaveAttribute('aria-haspopup', 'listbox')
  })

  it('renders density picker using BrandedSelect', async () => {
    render(<LocaleProvider><AppearanceSection /></LocaleProvider>)
    const trigger = await screen.findByTestId('appearance-density')
    expect(trigger).toHaveAttribute('aria-haspopup', 'listbox')
  })

  it('renders mode picker using BrandedSelect', async () => {
    render(<LocaleProvider><AppearanceSection /></LocaleProvider>)
    const trigger = await screen.findByTestId('appearance-mode')
    expect(trigger).toHaveAttribute('aria-haspopup', 'listbox')
  })

  it('renders UI font picker using BrandedSelect', async () => {
    render(<LocaleProvider><AppearanceSection /></LocaleProvider>)
    const trigger = await screen.findByTestId('appearance-uifont')
    expect(trigger).toHaveAttribute('aria-haspopup', 'listbox')
  })

  it('renders code font picker using BrandedSelect', async () => {
    render(<LocaleProvider><AppearanceSection /></LocaleProvider>)
    const trigger = await screen.findByTestId('appearance-codefont')
    expect(trigger).toHaveAttribute('aria-haspopup', 'listbox')
  })

  it('changing accent calls setThemeSettings and saveConfig', async () => {
    const save = vi.spyOn(system, 'saveConfig').mockResolvedValue({ ...baseCfg, accent: 'rose' })
    const apply = vi.spyOn(store, 'setThemeSettings').mockImplementation(() => {})
    render(<LocaleProvider><AppearanceSection /></LocaleProvider>)
    await pickOption('appearance-accent', 'Rose')
    await waitFor(() => {
      expect(apply).toHaveBeenCalledWith(expect.objectContaining({ accent: 'rose' }))
      expect(save).toHaveBeenCalledWith(expect.objectContaining({ accent: 'rose' }))
    })
  })

  it('changing mode calls setThemeSettings with {mode} only (not through ConfigDTO)', async () => {
    const apply = vi.spyOn(store, 'setThemeSettings').mockImplementation(() => {})
    vi.spyOn(system, 'saveConfig').mockResolvedValue(baseCfg)
    render(<LocaleProvider><AppearanceSection /></LocaleProvider>)
    await pickOption('appearance-mode', 'Dark')
    await waitFor(() => {
      expect(apply).toHaveBeenCalledWith({ mode: 'dark' })
    })
  })

  it('changing UI font calls setThemeSettings with {uiFont} only', async () => {
    const apply = vi.spyOn(store, 'setThemeSettings').mockImplementation(() => {})
    vi.spyOn(system, 'saveConfig').mockResolvedValue(baseCfg)
    render(<LocaleProvider><AppearanceSection /></LocaleProvider>)
    await pickOption('appearance-uifont', 'System')
    await waitFor(() => {
      expect(apply).toHaveBeenCalledWith({ uiFont: 'system' })
    })
  })

  describe('patch does not wipe theme or density', () => {
    it('changing accent does not wipe theme or density', async () => {
      vi.spyOn(system, 'saveConfig').mockResolvedValue({ ...baseCfg, accent: 'rose' })
      const apply = vi.spyOn(store, 'setThemeSettings').mockImplementation(() => {})
      render(<LocaleProvider><AppearanceSection /></LocaleProvider>)
      await pickOption('appearance-accent', 'Rose')
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
