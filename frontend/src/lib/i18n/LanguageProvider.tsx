import { useCallback, useEffect, useMemo, useState } from 'react'
import type { Lang } from './dictionaries'
import { translate } from './resolve'
import type { TKey, TVars } from './types'
import {
  DEFAULT_LANG,
  I18nContext,
  LANG_STORAGE_KEY,
  type I18nContextValue,
} from './context'

function isLang(value: string | null): value is Lang {
  return value === 'ro' || value === 'en'
}

/**
 * Pick the initial UI language: a previously persisted choice wins; otherwise
 * honor the browser preference on first visit (anything starting with `en` →
 * English), defaulting to RO. SSR-safe guards keep it from touching `window`
 * when it is not available.
 */
function detectInitialLang(): Lang {
  if (typeof window === 'undefined') return DEFAULT_LANG
  const stored = window.localStorage.getItem(LANG_STORAGE_KEY)
  if (isLang(stored)) return stored
  const nav = window.navigator.language?.toLowerCase() ?? ''
  if (nav.startsWith('en')) return 'en'
  if (nav.startsWith('ro')) return 'ro'
  return DEFAULT_LANG
}

/**
 * Holds the active dashboard UI language, persists it to localStorage, and
 * exposes `t()` + `{lang, setLang}` via context. Switching is instant (no
 * reload). This is the dashboard UI language only — the agent's conversation
 * language is decided server-side and is independent of this.
 */
export function LanguageProvider({ children }: { children: React.ReactNode }) {
  const [lang, setLangState] = useState<Lang>(detectInitialLang)

  // Reflect the language on <html lang> so the document + a11y tools agree.
  useEffect(() => {
    if (typeof document !== 'undefined') {
      document.documentElement.lang = lang
    }
  }, [lang])

  const setLang = useCallback((next: Lang) => {
    setLangState(next)
    if (typeof window !== 'undefined') {
      window.localStorage.setItem(LANG_STORAGE_KEY, next)
    }
  }, [])

  const value = useMemo<I18nContextValue>(
    () => ({
      lang,
      setLang,
      t: (key: TKey, vars?: TVars) => translate(lang, key, vars),
    }),
    [lang, setLang],
  )

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}
