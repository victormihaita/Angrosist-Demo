import { useEffect, useState } from 'react'
import { Search } from 'lucide-react'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useT, useEnums } from '@/lib/i18n'

const ALL = '__all__'

export interface CompanyFilterState {
  role: string
  country: string
  q: string
}

interface Props {
  value: CompanyFilterState
  onChange: (patch: Partial<CompanyFilterState>) => void
}

/**
 * Directory filter bar — mirrors LeadFilterBar: a debounced free-text input,
 * a role Select (array-containment on the backend), and a short country-code
 * input. Each change is lifted to the URL by the page.
 */
export function CompanyFilterBar({ value, onChange }: Props) {
  const { t } = useT()
  const { companyRoles } = useEnums()
  // Local mirrors so typing is responsive; debounce up to the URL.
  const [search, setSearch] = useState(value.q)
  const [country, setCountry] = useState(value.country)

  // Re-sync inputs when the URL changes externally (back/forward, reset).
  const [syncedQ, setSyncedQ] = useState(value.q)
  if (syncedQ !== value.q) {
    setSyncedQ(value.q)
    setSearch(value.q)
  }
  const [syncedCountry, setSyncedCountry] = useState(value.country)
  if (syncedCountry !== value.country) {
    setSyncedCountry(value.country)
    setCountry(value.country)
  }

  useEffect(() => {
    const handle = setTimeout(() => {
      if (search !== value.q) onChange({ q: search })
    }, 350)
    return () => clearTimeout(handle)
  }, [search, value.q, onChange])

  useEffect(() => {
    const handle = setTimeout(() => {
      const normalized = country.trim().toUpperCase()
      if (normalized !== value.country) onChange({ country: normalized })
    }, 350)
    return () => clearTimeout(handle)
  }, [country, value.country, onChange])

  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative flex-1 min-w-[200px]">
        <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t('companies.search')}
          className="pl-8"
          aria-label={t('companies.search')}
        />
      </div>

      <Select
        value={value.role || ALL}
        onValueChange={(v) => onChange({ role: v === ALL ? '' : v })}
      >
        <SelectTrigger className="w-[170px]" aria-label={t('companies.filterRole')}>
          <SelectValue placeholder={t('companies.filterRole')} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>{t('companies.allRoles')}</SelectItem>
          {companyRoles.map((r) => (
            <SelectItem key={r.value} value={r.value}>
              {r.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Input
        value={country}
        onChange={(e) => setCountry(e.target.value)}
        placeholder={t('companies.filterCountry')}
        className="w-[160px] uppercase"
        maxLength={2}
        aria-label={t('companies.filterCountry')}
      />
    </div>
  )
}
