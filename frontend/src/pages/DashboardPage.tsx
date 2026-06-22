import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { LeadTable } from '@/components/dashboard/LeadTable'
import { KpiCards } from '@/components/dashboard/KpiCards'
import {
  LeadFilterBar,
  UNASSIGNED,
  type FilterState,
} from '@/components/dashboard/LeadFilterBar'
import { useLeadsList, useUsers } from '@/hooks/useDashboard'
import { useT } from '@/lib/i18n'
import type { LeadFilters } from '@/lib/api'

const PAGE_SIZE = 25

export function DashboardPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const { t } = useT()
  const { data: users = [] } = useUsers()

  // The assignee filter is only meaningful when we can resolve names. Admins get
  // the full list; staff (403) get an empty list, so we hide the picker.
  const showAssignee = users.length > 0

  const filters: FilterState = useMemo(
    () => ({
      status: searchParams.get('status') ?? '',
      vertical: searchParams.get('vertical') ?? '',
      assigned_to: searchParams.get('assigned_to') ?? '',
      q: searchParams.get('q') ?? '',
    }),
    [searchParams],
  )

  // Cursor stack lives in the URL so Prev/Back works and pages are shareable.
  // `cursors` is a comma-joined list of cursors consumed so far; the current
  // page uses the last one.
  const cursorStack = useMemo(() => {
    const raw = searchParams.get('cursors')
    return raw ? raw.split(',').filter(Boolean) : []
  }, [searchParams])

  const currentCursor = cursorStack[cursorStack.length - 1]

  const queryFilters: LeadFilters = {
    status: filters.status || undefined,
    vertical: filters.vertical || undefined,
    assigned_to:
      filters.assigned_to === UNASSIGNED
        ? 'none'
        : filters.assigned_to || undefined,
    q: filters.q || undefined,
    cursor: currentCursor || undefined,
    limit: PAGE_SIZE,
  }

  const { data, isLoading, isFetching, error, refetch } =
    useLeadsList(queryFilters)

  // Changing a filter resets pagination (drop the cursor stack).
  const onFilterChange = useCallback(
    (patch: Partial<FilterState>) => {
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev)
        for (const [key, val] of Object.entries(patch)) {
          if (val) next.set(key, val)
          else next.delete(key)
        }
        next.delete('cursors')
        return next
      })
    },
    [setSearchParams],
  )

  const goNext = useCallback(() => {
    const nextCursor = data?.page.next_cursor
    if (!nextCursor) return
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      next.set('cursors', [...cursorStack, nextCursor].join(','))
      return next
    })
  }, [data?.page.next_cursor, cursorStack, setSearchParams])

  const goPrev = useCallback(() => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      const popped = cursorStack.slice(0, -1)
      if (popped.length) next.set('cursors', popped.join(','))
      else next.delete('cursors')
      return next
    })
  }, [cursorStack, setSearchParams])

  const hasPrev = cursorStack.length > 0
  const hasNext = !!data?.page.next_cursor

  return (
    <div className="max-w-7xl mx-auto px-4 py-8 w-full min-w-0">
      <div className="mb-6">
        <h1 className="text-xl font-semibold">{t('pipeline.title')}</h1>
        <p className="text-sm text-muted-foreground mt-1">
          {t('pipeline.subtitle')}
        </p>
      </div>

      <KpiCards />

      <div className="mb-4">
        <LeadFilterBar
          value={filters}
          users={users}
          showAssignee={showAssignee}
          onChange={onFilterChange}
        />
      </div>

      {isLoading && (
        <div className="rounded-lg border p-4 flex flex-col gap-3">
          {Array.from({ length: 8 }).map((_, i) => (
            <Skeleton key={i} className="h-9 w-full" />
          ))}
        </div>
      )}

      {error && !isLoading && (
        <div className="flex flex-col items-center gap-3 py-16">
          <p className="text-sm text-destructive">{t('pipeline.loadError')}</p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            {t('common.retry')}
          </Button>
        </div>
      )}

      {data && !isLoading && !error && (
        <>
          {data.data.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-20 text-muted-foreground text-sm gap-2">
              <span>{t('pipeline.empty')}</span>
            </div>
          ) : (
            <LeadTable leads={data.data} users={users} />
          )}

          <div className="flex items-center justify-between mt-4">
            <span className="text-xs text-muted-foreground">
              {t(
                data.page.count === 1 ? 'pipeline.countOne' : 'pipeline.countOther',
                { n: data.page.count },
              )}
            </span>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={goPrev}
                disabled={!hasPrev || isFetching}
              >
                {t('pipeline.prev')}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={goNext}
                disabled={!hasNext || isFetching}
              >
                {t('pipeline.next')}
              </Button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
