import { useNavigate } from 'react-router-dom'
import { AlertTriangle } from 'lucide-react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/dashboard/StatusBadge'
import { cn } from '@/lib/utils'
import { t } from '@/lib/strings'
import type { LeadSummary, PublicUser } from '@/lib/api'

interface Props {
  leads: LeadSummary[]
  users: PublicUser[]
}

function formatValue(v: number | null): string {
  if (v == null) return t.common.none
  return new Intl.NumberFormat('ro-RO', {
    style: 'currency',
    currency: 'RON',
    maximumFractionDigits: 0,
  }).format(v)
}

export function LeadTable({ leads, users }: Props) {
  const navigate = useNavigate()
  const userById = new Map(users.map((u) => [u.id, u.name || u.email]))

  return (
    <div className="rounded-lg border overflow-hidden">
      <div className="overflow-x-auto">
        <Table className="min-w-[860px]">
          <TableHeader>
            <TableRow>
              <TableHead>{t.pipeline.colCompany}</TableHead>
              <TableHead>{t.pipeline.colProduct}</TableHead>
              <TableHead>{t.pipeline.colQuantity}</TableHead>
              <TableHead>{t.pipeline.colLocation}</TableHead>
              <TableHead>{t.pipeline.colStatus}</TableHead>
              <TableHead>{t.pipeline.colAssignee}</TableHead>
              <TableHead className="text-right">{t.pipeline.colValue}</TableHead>
              <TableHead>{t.pipeline.colCreated}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {leads.map((lead) => (
              <TableRow
                key={lead.id}
                className={cn(
                  'cursor-pointer',
                  lead.needs_human && 'bg-destructive/5 hover:bg-destructive/10',
                )}
                onClick={() => navigate(`/dashboard/${lead.id}`)}
              >
                <TableCell className="font-medium">
                  <span className="flex items-center gap-1.5">
                    {lead.needs_human && (
                      <AlertTriangle
                        className="h-3.5 w-3.5 text-destructive shrink-0"
                        aria-label={t.pipeline.needsHuman}
                      />
                    )}
                    {lead.company_name || t.common.none}
                  </span>
                </TableCell>
                <TableCell>{lead.product_name || t.common.none}</TableCell>
                <TableCell className="whitespace-nowrap">
                  {lead.quantity != null
                    ? `${lead.quantity} ${lead.unit}`
                    : t.common.none}
                </TableCell>
                <TableCell>{lead.delivery_location || t.common.none}</TableCell>
                <TableCell>
                  <StatusBadge status={lead.status} />
                </TableCell>
                <TableCell className="whitespace-nowrap">
                  {lead.assigned_to ? (
                    userById.get(lead.assigned_to) ?? lead.assigned_to
                  ) : (
                    <Badge variant="outline" className="text-muted-foreground">
                      {t.pipeline.unassigned}
                    </Badge>
                  )}
                </TableCell>
                <TableCell className="text-right whitespace-nowrap tabular-nums">
                  {formatValue(lead.offer_value)}
                </TableCell>
                <TableCell className="text-muted-foreground text-xs whitespace-nowrap">
                  {new Date(lead.created_at).toLocaleDateString('ro-RO')}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}
