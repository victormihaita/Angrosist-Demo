import type { Lang } from './dictionaries'

/** BCP-47 locale tags per UI language, for Intl date/number formatting. */
const LOCALE_TAG: Record<Lang, string> = {
  ro: 'ro-RO',
  en: 'en-GB',
}

export function localeTag(lang: Lang): string {
  return LOCALE_TAG[lang]
}

/** Compact RON currency formatter, locale-aware. `none` is the empty marker. */
export function formatRON(
  lang: Lang,
  v: number | null | undefined,
  none = '—',
): string {
  if (v == null) return none
  return new Intl.NumberFormat(LOCALE_TAG[lang], {
    style: 'currency',
    currency: 'RON',
    maximumFractionDigits: 0,
  }).format(v)
}

export function formatDate(lang: Lang, iso: string): string {
  return new Date(iso).toLocaleDateString(LOCALE_TAG[lang])
}

export function formatDateTime(lang: Lang, iso: string): string {
  return new Date(iso).toLocaleString(LOCALE_TAG[lang])
}
