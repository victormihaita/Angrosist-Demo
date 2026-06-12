import { useNavigate } from 'react-router-dom'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import type { Lead } from '@/lib/api'

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  new: 'default',
  qualifying: 'secondary',
  confirmed: 'outline',
  failed: 'destructive',
}

const STATUS_LABEL: Record<string, string> = {
  new: 'Nou',
  qualifying: 'În calificare',
  confirmed: 'Confirmat',
  failed: 'Eșuat',
}

interface Props {
  leads: Lead[]
}

export function LeadTable({ leads }: Props) {
  const navigate = useNavigate()

  if (leads.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-muted-foreground text-sm gap-2">
        <span>Nu există lead-uri încă.</span>
        <a href="/" className="text-foreground underline underline-offset-2">Testează chat-ul →</a>
      </div>
    )
  }

  return (
    <div className="rounded-lg border overflow-hidden">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Companie</TableHead>
            <TableHead>CUI</TableHead>
            <TableHead>Produs</TableHead>
            <TableHead>Cantitate</TableHead>
            <TableHead>Locație</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Dată</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {leads.map((lead) => (
            <TableRow
              key={lead.id}
              className="cursor-pointer"
              onClick={() => navigate(`/dashboard/${lead.id}`)}
            >
              <TableCell className="font-medium">{lead.company_name || '—'}</TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground">{lead.cui || '—'}</TableCell>
              <TableCell>{lead.product_name || '—'}</TableCell>
              <TableCell>
                {lead.quantity != null ? `${lead.quantity} ${lead.unit}` : '—'}
              </TableCell>
              <TableCell>{lead.delivery_location || '—'}</TableCell>
              <TableCell>
                <Badge variant={STATUS_VARIANT[lead.status] ?? 'secondary'}>
                  {STATUS_LABEL[lead.status] ?? lead.status}
                </Badge>
              </TableCell>
              <TableCell className="text-muted-foreground text-xs">
                {new Date(lead.created_at).toLocaleDateString('ro-RO')}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
