import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ModelSwitcher } from './ModelSwitcher'
import * as api from '../lib/api'
import { LocaleProvider } from '../lib/i18n'
import type { ModelInfo } from '../lib/api'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

const descriptorModels: ModelInfo[] = [
  {
    id: 'deepseek-v4-flash',
    label: 'DeepSeek V4 Flash',
    provider: 'deepseek',
    caps: { vision: false, tools: true, reasoning: true },
    effort: { kind: 'levels', levels: ['low', 'medium', 'high', 'max'], default: 'max' },
    context: 1_000_000,
  },
  {
    id: 'deepseek-v4-pro',
    label: 'DeepSeek V4 Pro',
    provider: 'deepseek',
    caps: { vision: false, tools: true, reasoning: true },
    effort: { kind: 'levels', levels: ['low', 'medium', 'high', 'max'], default: 'max' },
    context: 1_000_000,
  },
]

describe('ModelSwitcher', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('opens the menu and lists models from fetchModels', async () => {
    vi.spyOn(api, 'fetchModels').mockResolvedValue([{ id: 'pro', label: 'DeepSeek Pro' }, { id: 'flash', label: 'Flash' }])
    wrap(<ModelSwitcher label="DeepSeek Pro" activeId="pro" onPick={() => {}} />)
    await userEvent.click(screen.getByTestId('model-trigger'))
    expect(await screen.findByText('Flash')).toBeInTheDocument()
  })

  it('picking a model fires onPick(id) and closes', async () => {
    vi.spyOn(api, 'fetchModels').mockResolvedValue([{ id: 'flash', label: 'Flash' }])
    const picked: string[] = []
    wrap(<ModelSwitcher label="DeepSeek Pro" activeId="pro" onPick={(id) => picked.push(id)} />)
    await userEvent.click(screen.getByTestId('model-trigger'))
    await userEvent.click(await screen.findByText('Flash'))
    expect(picked).toEqual(['flash'])
  })

  it('marks the active model as aria-selected using id not label', async () => {
    vi.spyOn(api, 'fetchModels').mockResolvedValue([
      { id: 'pro', label: 'DeepSeek Pro' },
      { id: 'flash', label: 'Flash' },
    ])
    wrap(<ModelSwitcher label="DeepSeek Pro" activeId="pro" onPick={() => {}} />)
    await userEvent.click(screen.getByTestId('model-trigger'))
    const items = await screen.findAllByRole('option')
    expect(items[0]).toHaveAttribute('aria-selected', 'true')
    expect(items[1]).toHaveAttribute('aria-selected', 'false')
  })

  it('option items are div elements (not button) to satisfy ARIA spec', async () => {
    vi.spyOn(api, 'fetchModels').mockResolvedValue([{ id: 'pro', label: 'DeepSeek Pro' }])
    wrap(<ModelSwitcher label="DeepSeek Pro" activeId="pro" onPick={() => {}} />)
    await userEvent.click(screen.getByTestId('model-trigger'))
    const item = await screen.findByRole('option', { name: /DeepSeek Pro/i })
    expect(item.tagName.toLowerCase()).toBe('div')
  })

  // New tests for capability-rich rows

  it('uses models prop without calling fetchModels when models prop is non-empty', async () => {
    const fetchSpy = vi.spyOn(api, 'fetchModels').mockResolvedValue([])
    wrap(
      <ModelSwitcher
        label="DeepSeek V4 Flash"
        activeId="deepseek-v4-flash"
        models={descriptorModels}
        onPick={() => {}}
      />,
    )
    await userEvent.click(screen.getByTestId('model-trigger'))
    // Both models should appear as option rows
    expect(await screen.findAllByRole('option')).toHaveLength(2)
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('renders context badge [data-testid="model-ctx"] showing "1M" for context:1000000', async () => {
    vi.spyOn(api, 'fetchModels').mockResolvedValue([])
    wrap(
      <ModelSwitcher
        label="DeepSeek V4 Flash"
        activeId="deepseek-v4-flash"
        models={descriptorModels}
        onPick={() => {}}
      />,
    )
    await userEvent.click(screen.getByTestId('model-trigger'))
    const badges = await screen.findAllByTestId('model-ctx')
    expect(badges.length).toBeGreaterThan(0)
    expect(badges[0]).toHaveTextContent('1M')
  })

  it('renders reasoning glyph [data-testid="cap-reasoning"] when caps.reasoning is true', async () => {
    vi.spyOn(api, 'fetchModels').mockResolvedValue([])
    wrap(
      <ModelSwitcher
        label="DeepSeek V4 Flash"
        activeId="deepseek-v4-flash"
        models={descriptorModels}
        onPick={() => {}}
      />,
    )
    await userEvent.click(screen.getByTestId('model-trigger'))
    const glyphs = await screen.findAllByTestId('cap-reasoning')
    expect(glyphs.length).toBeGreaterThan(0)
  })

  it('renders tools glyph [data-testid="cap-tools"] when caps.tools is true', async () => {
    vi.spyOn(api, 'fetchModels').mockResolvedValue([])
    wrap(
      <ModelSwitcher
        label="DeepSeek V4 Flash"
        activeId="deepseek-v4-flash"
        models={descriptorModels}
        onPick={() => {}}
      />,
    )
    await userEvent.click(screen.getByTestId('model-trigger'))
    const glyphs = await screen.findAllByTestId('cap-tools')
    expect(glyphs.length).toBeGreaterThan(0)
  })

  it('back-compat: with no models prop it self-fetches on open', async () => {
    const fetchSpy = vi.spyOn(api, 'fetchModels').mockResolvedValue([{ id: 'flash', label: 'Flash' }, { id: 'pro', label: 'Pro' }])
    wrap(<ModelSwitcher label="Model" activeId="flash" onPick={() => {}} />)
    await userEvent.click(screen.getByTestId('model-trigger'))
    expect(await screen.findAllByRole('option')).toHaveLength(2)
    expect(fetchSpy).toHaveBeenCalledOnce()
  })
})
