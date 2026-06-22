import { useNavigate } from 'react-router-dom'
import { ArrowRight, Inbox } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { StatusBadge } from '@/components/dashboard/StatusBadge'
import { useHandoffs } from '@/hooks/useDashboard'
import { useT, formatDateTime } from '@/lib/i18n'

export function HandoffsPage() {
  const navigate = useNavigate()
  const { t, lang } = useT()
  const { data, isLoading, error, refetch } = useHandoffs()

  return (
    <div className="max-w-7xl mx-auto px-4 py-8 w-full min-w-0">
      <div className="mb-6">
        <h1 className="text-xl font-semibold">{t('handoffs.title')}</h1>
        <p className="text-sm text-muted-foreground mt-1">
          {t('handoffs.subtitle')}
        </p>
      </div>

      {isLoading && (
        <div className="rounded-lg border p-4 flex flex-col gap-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-9 w-full" />
          ))}
        </div>
      )}

      {error && !isLoading && (
        <div className="flex flex-col items-center gap-3 py-16">
          <p className="text-sm text-destructive">{t('handoffs.loadError')}</p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            {t('common.retry')}
          </Button>
        </div>
      )}

      {data && !isLoading && !error && (
        <>
          {data.data.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-20 text-muted-foreground text-sm gap-3">
              <Inbox className="h-8 w-8 opacity-50" />
              <span>{t('handoffs.empty')}</span>
            </div>
          ) : (
            <div className="rounded-lg border overflow-hidden">
              <div className="overflow-x-auto">
                <Table className="min-w-[820px]">
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('handoffs.colCompany')}</TableHead>
                      <TableHead>{t('handoffs.colProduct')}</TableHead>
                      <TableHead>{t('pipeline.colStatus')}</TableHead>
                      <TableHead>{t('handoffs.colSnippet')}</TableHead>
                      <TableHead>{t('handoffs.colCreated')}</TableHead>
                      <TableHead className="text-right" />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {data.data.map((h) => (
                      <TableRow
                        key={h.id}
                        className="cursor-pointer bg-destructive/5 hover:bg-destructive/10"
                        onClick={() => navigate(`/dashboard/${h.id}`)}
                      >
                        <TableCell className="font-medium">
                          {h.company_name || t('common.none')}
                        </TableCell>
                        <TableCell>
                          {h.product_name || t('common.none')}
                        </TableCell>
                        <TableCell>
                          <StatusBadge status={h.status} />
                        </TableCell>
                        <TableCell className="max-w-[320px]">
                          <span className="block truncate text-muted-foreground">
                            {h.last_message || t('common.none')}
                          </span>
                        </TableCell>
                        <TableCell className="text-muted-foreground text-xs whitespace-nowrap">
                          {formatDateTime(lang, h.created_at)}
                        </TableCell>
                        <TableCell className="text-right">
                          <span className="inline-flex items-center gap-1 text-xs font-medium text-primary">
                            {t('handoffs.open')}
                            <ArrowRight className="h-3.5 w-3.5" />
                          </span>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}
