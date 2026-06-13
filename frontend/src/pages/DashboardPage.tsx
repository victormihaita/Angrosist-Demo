import { Loader2 } from 'lucide-react'
import { LeadTable } from '@/components/dashboard/LeadTable'
import { useLeads } from '@/hooks/useLeads'

export function DashboardPage() {
  const { data: leads, isLoading, error } = useLeads()

  return (
    <div className="max-w-7xl mx-auto px-4 py-8 min-w-0 overflow-x-hidden">
      <div className="mb-6">
        <h1 className="text-xl font-semibold">Lead-uri</h1>
        <p className="text-sm text-muted-foreground mt-1">
          Cumpărători calificați prin conversație cu agentul Angrosist
        </p>
      </div>

      {isLoading && (
        <div className="flex items-center gap-2 text-muted-foreground text-sm py-12 justify-center">
          <Loader2 className="h-4 w-4 animate-spin" />
          Se încarcă...
        </div>
      )}

      {error && (
        <p className="text-sm text-destructive py-8 text-center">
          Eroare la încărcarea lead-urilor.
        </p>
      )}

      {leads && <LeadTable leads={leads} />}
    </div>
  )
}
