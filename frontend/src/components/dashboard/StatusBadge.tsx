import { Badge } from '@/components/ui/badge'
import { statusLabel } from '@/lib/strings'

const STATUS_VARIANT: Record<
  string,
  'default' | 'secondary' | 'destructive' | 'outline'
> = {
  new: 'default',
  qualifying: 'secondary',
  needs_human: 'destructive',
  qualified: 'secondary',
  offer_requested: 'outline',
  offer_sent: 'outline',
  negotiation: 'outline',
  won: 'default',
  lost: 'destructive',
}

export function StatusBadge({ status }: { status: string }) {
  return (
    <Badge variant={STATUS_VARIANT[status] ?? 'secondary'}>
      {statusLabel(status)}
    </Badge>
  )
}
