import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { assignLead, type AuthedLeadDetail, type PublicUser } from '@/lib/api'
import { useAuth } from '@/auth/useAuth'
import { useT } from '@/lib/i18n'

const UNASSIGNED = '__unassigned__'

interface Props {
  lead: AuthedLeadDetail
  users: PublicUser[]
}

export function AssigneeCard({ lead, users }: Props) {
  const queryClient = useQueryClient()
  const { user } = useAuth()
  const { t } = useT()

  const mutation = useMutation({
    mutationFn: (userId: string | null) => assignLead(lead.id, userId),
    onMutate: async (userId) => {
      await queryClient.cancelQueries({ queryKey: ['lead', lead.id] })
      const prev = queryClient.getQueryData<AuthedLeadDetail>(['lead', lead.id])
      queryClient.setQueryData<AuthedLeadDetail>(['lead', lead.id], (old) =>
        old ? { ...old, assigned_to: userId } : old,
      )
      return { prev }
    },
    onError: (_err, _v, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(['lead', lead.id], ctx.prev)
      toast.error(t('detail.assignError'))
    },
    onSuccess: () => toast.success(t('detail.assignSaved')),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['lead', lead.id] })
      queryClient.invalidateQueries({ queryKey: ['leads'] })
    },
  })

  const busy = mutation.isPending
  const hasUserList = users.length > 0

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
          {t('detail.assignee')}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {hasUserList ? (
          <Select
            value={lead.assigned_to ?? UNASSIGNED}
            onValueChange={(v) =>
              mutation.mutate(v === UNASSIGNED ? null : v)
            }
            disabled={busy}
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={UNASSIGNED}>
                {t('pipeline.unassigned')}
              </SelectItem>
              {users.map((u) => (
                <SelectItem key={u.id} value={u.id}>
                  {u.name || u.email}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          // Staff can't list users (403); degrade to self-assign / unassign.
          <div className="flex flex-col gap-2">
            <p className="text-sm text-muted-foreground">
              {lead.assigned_to
                ? lead.assigned_to === user?.id
                  ? user?.name || user?.email
                  : lead.assigned_to
                : t('pipeline.unassigned')}
            </p>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="outline"
                disabled={busy || !user || lead.assigned_to === user.id}
                onClick={() => user && mutation.mutate(user.id)}
              >
                {t('detail.assignToMe')}
              </Button>
              {lead.assigned_to && (
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy}
                  onClick={() => mutation.mutate(null)}
                >
                  {t('pipeline.unassigned')}
                </Button>
              )}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
