import { useContext, useMemo } from 'react'
import { I18nContext } from './context'
import type { Lang } from './dictionaries'
import { translate } from './resolve'
import type { TKey } from './types'

/**
 * Access the active language + translate function. Throws if used outside
 * <LanguageProvider> so a missing provider fails loudly in development.
 */
export function useT() {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('useT must be used within <LanguageProvider>')
  return ctx
}

/** The canonical lead statuses in pipeline order. */
const STATUS_VALUES = [
  'new',
  'qualifying',
  'needs_human',
  'qualified',
  'offer_requested',
  'offer_sent',
  'negotiation',
  'won',
  'lost',
] as const

const VERTICAL_VALUES = ['angrosist', 'palletclearance', 'skalyou'] as const

/** Company roles in directory order, used for the role filter Select. */
const ROLE_VALUES = [
  'distributor',
  'importer',
  'wholesaler',
  'retailer',
  'horeca',
  'processor',
  'producer',
  'buyer',
  'seller',
] as const

export interface Option {
  value: string
  label: string
}

function options(lang: Lang, prefix: string, values: readonly string[]): Option[] {
  return values.map((value) => ({
    value,
    label: translate(lang, `${prefix}.${value}` as TKey),
  }))
}

/**
 * Localized enum option lists + label helpers, derived from the active language.
 * Enum VALUES stay the canonical backend codes; only the labels are localized.
 */
export function useEnums() {
  const { lang } = useT()
  return useMemo(
    () => ({
      leadStatuses: options(lang, 'status', STATUS_VALUES),
      verticals: options(lang, 'vertical', VERTICAL_VALUES),
      companyRoles: options(lang, 'role', ROLE_VALUES),
      statusLabel: (status: string) =>
        translate(lang, `status.${status}` as TKey),
      verticalLabel: (v: string) => translate(lang, `vertical.${v}` as TKey),
      roleLabel: (r: string) => translate(lang, `role.${r}` as TKey),
    }),
    [lang],
  )
}
