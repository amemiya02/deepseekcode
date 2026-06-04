import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { ModelSwitcher } from './ModelSwitcher'
import * as api from '../lib/api'
import { LocaleProvider } from '../lib/i18n'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('ModelSwitcher', () => {
  it('opens the menu and lists models from fetchModels', async () => {
    vi.spyOn(api, 'fetchModels').mockResolvedValue([{ id: 'pro', label: 'DeepSeek Pro' }, { id: 'flash', label: 'Flash' }])
    wrap(<ModelSwitcher label="DeepSeek Pro" onPick={() => {}} />)
    await userEvent.click(screen.getByTestId('model-trigger'))
    expect(await screen.findByText('Flash')).toBeInTheDocument()
  })

  it('picking a model fires onPick(id) and closes', async () => {
    vi.spyOn(api, 'fetchModels').mockResolvedValue([{ id: 'flash', label: 'Flash' }])
    const picked: string[] = []
    wrap(<ModelSwitcher label="DeepSeek Pro" onPick={(id) => picked.push(id)} />)
    await userEvent.click(screen.getByTestId('model-trigger'))
    await userEvent.click(await screen.findByText('Flash'))
    expect(picked).toEqual(['flash'])
  })
})
