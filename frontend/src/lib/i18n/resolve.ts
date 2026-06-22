import { dictionaries, type Lang } from './dictionaries'
import type { TKey, TVars } from './types'

/**
 * Resolve a dotted key against the active locale, falling back to RO (the
 * canonical dictionary) if a leaf is somehow missing, then to the key itself.
 * Fills `{var}` placeholders from `vars`.
 *
 * This is a pure function so it can be reused outside React (e.g. building enum
 * label lists) given an explicit lang.
 */
export function translate(lang: Lang, key: TKey, vars?: TVars): string {
  const value = lookup(lang, key) ?? lookup('ro', key) ?? key
  return vars ? interpolate(value, vars) : value
}

function lookup(lang: Lang, key: string): string | undefined {
  let node: unknown = dictionaries[lang]
  for (const part of key.split('.')) {
    if (node && typeof node === 'object' && part in node) {
      node = (node as Record<string, unknown>)[part]
    } else {
      return undefined
    }
  }
  return typeof node === 'string' ? node : undefined
}

function interpolate(template: string, vars: TVars): string {
  return template.replace(/\{(\w+)\}/g, (match, name: string) =>
    name in vars ? String(vars[name]) : match,
  )
}
