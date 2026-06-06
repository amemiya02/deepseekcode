// Adapted from deepseek-reasonix (MIT) — lib/i18n.tsx.
// i18n is the desktop's localization seam. The active locale lives in React state
// behind a context — flipping it re-renders the whole tree (App is a child of the
// provider). A module-level mirror (currentLocale) lets non-React code translate
// too; it stays fresh because the provider updates it on every render.
//
// Contract 3: t(key: string, fallback?: string, vars?) — plain-string keys (NOT a
// compile-enforced union); resolution is bundle[locale][key] -> fallback -> key.
import { createContext, useCallback, useContext, useState } from 'react'
import type { ReactNode } from 'react'
import { en } from '../locales/en'
import { zhCN } from '../locales/zh'

export type Locale = 'en' | 'zh-CN'

const BUNDLES: Record<Locale, Record<string, string>> = { en, 'zh-CN': zhCN }
const STORAGE_KEY = 'dsc.locale'

// currentLocale mirrors the active locale for callers outside React.
let currentLocale: Locale = initialLocale()

function initialLocale(): Locale {
  try {
    const v = typeof localStorage !== 'undefined' ? localStorage.getItem(STORAGE_KEY) : null
    if (v === 'en' || v === 'zh-CN') return v
  } catch {
    /* ignore */
  }
  return 'en'
}

function interpolate(s: string, vars?: Record<string, string | number>): string {
  if (!vars) return s
  return s.replace(/\{(\w+)\}/g, (_, k: string) => (vars[k] !== undefined ? String(vars[k]) : `{${k}}`))
}

// translate resolves a key for a locale: bundle[locale][key] -> fallback -> key.
function translate(
  locale: Locale,
  key: string,
  fallback?: string,
  vars?: Record<string, string | number>,
): string {
  const raw = BUNDLES[locale][key] ?? fallback ?? key
  return interpolate(raw, vars)
}

// t is the non-reactive translator for code outside React. It reads the module
// mirror, which the provider keeps in sync.
export function t(key: string, fallback?: string, vars?: Record<string, string | number>): string {
  return translate(currentLocale, key, fallback, vars)
}

export function getLocale(): Locale {
  return currentLocale
}

export function setLocale(next: Locale): void {
  currentLocale = next
  try {
    localStorage.setItem(STORAGE_KEY, next)
  } catch {
    /* ignore */
  }
}

export type Translator = (key: string, fallback?: string, vars?: Record<string, string | number>) => string

interface I18nValue {
  locale: Locale
  setLocale: (next: Locale) => void
  t: Translator
}

const I18nContext = createContext<I18nValue | null>(null)

export function LocaleProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(() => initialLocale())
  currentLocale = locale // keep the mirror fresh for non-React callers

  const switchLocale = useCallback((next: Locale) => {
    setLocale(next) // persists + updates the module mirror
    setLocaleState(next) // re-renders the tree
  }, [])

  const tt = useCallback<Translator>((key, fallback, vars) => translate(locale, key, fallback, vars), [locale])

  return (
    <I18nContext.Provider value={{ locale, setLocale: switchLocale, t: tt }}>
      {children}
    </I18nContext.Provider>
  )
}

export function useLocale(): I18nValue {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('useLocale must be used within a LocaleProvider')
  return ctx
}

// useT is the common shorthand: just the translator.
export function useT(): Translator {
  return useLocale().t
}
