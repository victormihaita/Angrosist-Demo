/**
 * Dependency-free i18n layer (RO default, EN available). A small typed
 * dictionary + context hook, consistent with the project's no-extra-deps stance.
 *
 * - `LanguageProvider` wraps the app and persists the choice to localStorage.
 * - `useT()` returns `{ t, lang, setLang }`; `t(key, vars?)` is typed to the RO
 *   dictionary shape, so missing keys are compile errors.
 * - `useEnums()` gives localized status/vertical/role labels + option lists
 *   (values stay the canonical backend codes).
 * - `formatRON` / `formatDate` / `formatDateTime` are locale-aware Intl helpers.
 *
 * NOTE: this is the dashboard UI language only. The agent's conversation
 * language is decided server-side and is independent (see frontend/CLAUDE.md).
 */
export { LanguageProvider } from './LanguageProvider'
export { useT, useEnums, type Option } from './useT'
export { formatRON, formatDate, formatDateTime, localeTag } from './format'
export type { Lang } from './dictionaries'
export type { TKey, TVars } from './types'
