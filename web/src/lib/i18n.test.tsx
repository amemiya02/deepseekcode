import { render, screen, act } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { t, setLocale, getLocale, LocaleProvider, useLocale, useT } from './i18n'
import { en } from '../locales/en'
import { zhCN } from '../locales/zh'

beforeEach(() => {
  localStorage.clear()
  setLocale('en')
})

describe('i18n', () => {
  it('t() returns the English string for a known key', () => {
    expect(t('app.newSession')).toBe(en['app.newSession'])
  })

  it('t() switches with the active locale (module mirror)', () => {
    setLocale('zh-CN')
    expect(t('app.newSession')).toBe(zhCN['app.newSession'])
    expect(getLocale()).toBe('zh-CN')
  })

  it('t() interpolates {var} placeholders', () => {
    // en['palette.results'] must be "{n} results"
    expect(t('palette.results', undefined, { n: 3 })).toBe('3 results')
  })

  it('t() returns the fallback for an unknown key', () => {
    expect(t('does.not.exist', 'Fallback')).toBe('Fallback')
  })

  it('t() returns the key itself when there is no entry and no fallback', () => {
    expect(t('does.not.exist')).toBe('does.not.exist')
  })

  it('en and zh-CN have identical key sets (runtime parity check)', () => {
    expect(Object.keys(zhCN).sort()).toEqual(Object.keys(en).sort())
  })

  it('no zh-CN value is empty (every key is actually translated)', () => {
    for (const [k, v] of Object.entries(zhCN)) {
      expect(v.length, `${k} is empty`).toBeGreaterThan(0)
    }
  })

  it('useLocale re-renders consumers on locale switch', () => {
    function Probe() {
      const tt = useT()
      return <span data-testid="probe">{tt('app.newSession')}</span>
    }
    function Switcher() {
      const { setLocale: set } = useLocale()
      return <button onClick={() => set('zh-CN')}>switch</button>
    }
    render(
      <LocaleProvider>
        <Probe />
        <Switcher />
      </LocaleProvider>,
    )
    expect(screen.getByTestId('probe').textContent).toBe(en['app.newSession'])
    act(() => screen.getByText('switch').click())
    expect(screen.getByTestId('probe').textContent).toBe(zhCN['app.newSession'])
  })
})
