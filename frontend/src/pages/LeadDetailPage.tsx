import { useParams, useNavigate } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { LeadDetail } from '@/components/dashboard/LeadDetail'
import { useLead } from '@/hooks/useLeads'

export function LeadDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data: lead, isLoading, error } = useLead(id ?? '')

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground text-sm py-20 justify-center">
        <Loader2 className="h-4 w-4 animate-spin" />
        Se încarcă...
      </div>
    )
  }

  if (error || !lead) {
    return (
      <div className="flex flex-col items-center gap-3 py-20">
        <p className="text-sm text-destructive">Lead negăsit.</p>
        <button
          className="text-sm underline underline-offset-2"
          onClick={() => navigate('/dashboard')}
        >
          Înapoi la dashboard
        </button>
      </div>
    )
  }

  return (
    <div className="max-w-7xl mx-auto px-4 py-8 flex-1">
      <LeadDetail lead={lead} />
    </div>
  )
}
