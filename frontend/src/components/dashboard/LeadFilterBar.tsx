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
import { t, LEAD_STATUSES, VERTICALS } from '@/lib/strings'
import type { PublicUser } from '@/lib/api'

const ALL = '__all__'
const UNASSIGNED = '__unassigned__'

export interface FilterState {
  status: string
  vertical: string
  assigned_to: string
  q: string
}

interface Props {
  value: FilterState
  users: PublicUser[]
  showAssignee: boolean
  onChange: (patch: Partial<FilterState>) => void
}

export function LeadFilterBar({ value, users, showAssignee, onChange }: Props) {
  // Local mirror so typing in search is responsive; debounce up to the URL.
  const [search, setSearch] = useState(value.q)

  // Re-sync the input when the URL `q` changes externally (back/forward, reset)
  // via the render-time pattern rather than an effect.
  const [syncedQ, setSyncedQ] = useState(value.q)
  if (syncedQ !== value.q) {
    setSyncedQ(value.q)
    setSearch(value.q)
  }

  useEffect(() => {
    const handle = setTimeout(() => {
      if (search !== value.q) onChange({ q: search })
    }, 350)
    return () => clearTimeout(handle)
  }, [search, value.q, onChange])

  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative flex-1 min-w-[200px]">
        <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t.pipeline.search}
          className="pl-8"
          aria-label={t.pipeline.search}
        />
      </div>

      <Select
        value={value.status || ALL}
        onValueChange={(v) => onChange({ status: v === ALL ? '' : v })}
      >
        <SelectTrigger className="w-[170px]" aria-label={t.pipeline.filterStatus}>
          <SelectValue placeholder={t.pipeline.filterStatus} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>{t.pipeline.allStatuses}</SelectItem>
          {LEAD_STATUSES.map((s) => (
            <SelectItem key={s.value} value={s.value}>
              {s.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={value.vertical || ALL}
        onValueChange={(v) => onChange({ vertical: v === ALL ? '' : v })}
      >
        <SelectTrigger className="w-[170px]" aria-label={t.pipeline.filterVertical}>
          <SelectValue placeholder={t.pipeline.filterVertical} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>{t.pipeline.allVerticals}</SelectItem>
          {VERTICALS.map((v) => (
            <SelectItem key={v.value} value={v.value}>
              {v.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {showAssignee && (
        <Select
          value={value.assigned_to || ALL}
          onValueChange={(v) =>
            onChange({ assigned_to: v === ALL ? '' : v })
          }
        >
          <SelectTrigger
            className="w-[180px]"
            aria-label={t.pipeline.filterAssignee}
          >
            <SelectValue placeholder={t.pipeline.filterAssignee} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>{t.pipeline.allAssignees}</SelectItem>
            <SelectItem value={UNASSIGNED}>{t.pipeline.unassigned}</SelectItem>
            {users.map((u) => (
              <SelectItem key={u.id} value={u.id}>
                {u.name || u.email}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    </div>
  )
}

export { UNASSIGNED }
