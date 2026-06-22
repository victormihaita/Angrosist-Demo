import { useParams, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { LeadDetail } from '@/components/dashboard/LeadDetail'
import { useLeadDetail, useUsers } from '@/hooks/useDashboard'
import { useT } from '@/lib/i18n'

export function LeadDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { t } = useT()
  const { data: lead, isLoading, error, refetch } = useLeadDetail(id ?? '')
  const { data: users = [] } = useUsers()

  if (isLoading) {
    return (
      <div className="max-w-7xl mx-auto px-4 py-8 w-full">
        <Skeleton className="h-8 w-64 mb-6" />
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <div className="flex flex-col gap-6">
            <Skeleton className="h-40 w-full" />
            <Skeleton className="h-72 w-full" />
          </div>
          <div className="flex flex-col gap-6">
            <Skeleton className="h-56 w-full" />
            <Skeleton className="h-40 w-full" />
            <Skeleton className="h-64 w-full" />
          </div>
        </div>
      </div>
    )
  }

  if (error || !lead) {
    return (
      <div className="flex flex-col items-center gap-3 py-20">
        <p className="text-sm text-destructive">{t('detail.notFound')}</p>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            {t('common.retry')}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => navigate('/dashboard')}>
            {t('common.back')}
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="max-w-7xl mx-auto px-4 py-8 w-full min-w-0">
      <LeadDetail lead={lead} users={users} />
    </div>
  )
}
