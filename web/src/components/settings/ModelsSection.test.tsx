import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { LocaleProvider } from '../../lib/i18n'
import { ModelsSection } from './ModelsSection'
import * as useConfigMod from '../../lib/useConfig'

afterEach(() => vi.restoreAllMocks())

describe('ModelsSection', () => {
  it('renders the model picker as a BrandedSelect from /v1/models', async () => {
    vi.spyOn(useConfigMod, 'useConfig').mockReturnValue({
      cfg: { model: 'deepseek-v4-flash', reasoningEffort: 'max', autoRoute: false, escalationModel: '', autoReasoning: false } as never,
      error: '', reload: () => {}, clearError: () => {}, patch: vi.fn(),
    })
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true, json: async () => ({ active: 'deepseek-v4-flash', effort: 'max',
        models: [{ id: 'deepseek-v4-flash', label: 'DeepSeek V4 Flash' }, { id: 'deepseek-v4-pro', label: 'DeepSeek V4 Pro' }] }),
    } as Response)
    render(<LocaleProvider><ModelsSection /></LocaleProvider>)
    await waitFor(() => expect(screen.getByTestId('models-model')).toHaveAttribute('aria-haspopup', 'listbox'))
    expect(screen.getByTestId('models-autoreasoning')).toHaveAttribute('role', 'switch')
  })
})
