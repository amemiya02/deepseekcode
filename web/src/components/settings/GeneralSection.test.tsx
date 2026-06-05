import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { LocaleProvider, setLocale } from '../../lib/i18n'
import { GeneralSection } from './GeneralSection'
import * as system from '../../lib/system'

beforeEach(() => {
  // Reset locale to English before each test so tests are independent
  setLocale('en')
})

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

describe('GeneralSection language', () => {
  it('changing language to 中文 flips the document locale to zh-CN', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg, language: 'en' })
    vi.spyOn(system, 'saveConfig').mockResolvedValue({ ...baseCfg, language: 'zh' })
    render(<LocaleProvider><GeneralSection /></LocaleProvider>)
    const select = await screen.findByDisplayValue('English')
    fireEvent.change(select, { target: { value: 'zh' } })
    // After locale flips to zh-CN the reactive label re-renders in Chinese.
    await waitFor(() => expect(screen.getByText('语言')).toBeInTheDocument())
  })
})

describe('GeneralSection', () => {
  it('loads config and shows the language + verbosity controls', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg })
    render(<LocaleProvider><GeneralSection /></LocaleProvider>)
    await waitFor(() => expect(screen.getByLabelText(/language/i)).toBeInTheDocument())
    const verbosity = screen.getByLabelText(/verbosity/i) as HTMLSelectElement
    expect(['normal', 'verbose', 'summary']).toContain(verbosity.value)
  })

  it('saves the verbosity change via saveConfig', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg })
    const save = vi.spyOn(system, 'saveConfig').mockResolvedValue({ ...baseCfg, transcriptVerbosity: 'verbose' })
    render(<LocaleProvider><GeneralSection /></LocaleProvider>)
    const verbosity = (await screen.findByLabelText(/verbosity/i)) as HTMLSelectElement
    await userEvent.selectOptions(verbosity, 'verbose')
    await waitFor(() => expect(save).toHaveBeenCalledWith(expect.objectContaining({ transcriptVerbosity: 'verbose' })))
  })
})
