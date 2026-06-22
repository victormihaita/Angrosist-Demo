import { createContext } from 'react'
import type { Lang } from './dictionaries'
import type { TKey, TVars } from './types'

export interface I18nContextValue {
  lang: Lang
  setLang: (lang: Lang) => void
  /** Translate a typed dotted key with optional `{var}` interpolation. */
  t: (key: TKey, vars?: TVars) => string
}

export const I18nContext = createContext<I18nContextValue | null>(null)

export const LANG_STORAGE_KEY = 'angrosist_lang'
export const DEFAULT_LANG: Lang = 'ro'
