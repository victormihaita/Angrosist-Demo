import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { updateOffer, type AuthedLeadDetail, type OfferUpdate } from '@/lib/api'
import { useT, useEnums } from '@/lib/i18n'

interface Props {
  lead: AuthedLeadDetail
}

export function OfferCard({ lead }: Props) {
  const queryClient = useQueryClient()
  const { t } = useT()
  const { leadStatuses } = useEnums()
  const [status, setStatus] = useState(lead.status)
  const [value, setValue] = useState(
    lead.offer_value != null ? String(lead.offer_value) : '',
  )
  const [note, setNote] = useState(lead.offer_note ?? '')

  // Reset the form when the underlying lead changes — the React-recommended
  // "adjust state during render" pattern (no effect) keyed on a server-state
  // fingerprint, so a refetch/optimistic update re-seeds the inputs.
  const fingerprint = `${lead.id}:${lead.status}:${lead.offer_value}:${lead.offer_note}`
  const [seen, setSeen] = useState(fingerprint)
  if (seen !== fingerprint) {
    setSeen(fingerprint)
    setStatus(lead.status)
    setValue(lead.offer_value != null ? String(lead.offer_value) : '')
    setNote(lead.offer_note ?? '')
  }

  const mutation = useMutation({
    mutationFn: (body: OfferUpdate) => updateOffer(lead.id, body),
    onMutate: async (body) => {
      await queryClient.cancelQueries({ queryKey: ['lead', lead.id] })
      const prev = queryClient.getQueryData<AuthedLeadDetail>(['lead', lead.id])
      queryClient.setQueryData<AuthedLeadDetail>(['lead', lead.id], (old) =>
        old
          ? {
              ...old,
              status: body.status ?? old.status,
              offer_value: body.value ?? old.offer_value,
              offer_note: body.note ?? old.offer_note,
            }
          : old,
      )
      return { prev }
    },
    onError: (_err, _body, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(['lead', lead.id], ctx.prev)
      toast.error(t('detail.offerError'))
    },
    onSuccess: () => {
      toast.success(t('detail.offerSaved'))
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['lead', lead.id] })
      queryClient.invalidateQueries({ queryKey: ['leads'] })
    },
  })

  function onSave() {
    const body: OfferUpdate = { status }
    const parsed = value.trim() === '' ? undefined : Number(value)
    if (parsed != null && !Number.isNaN(parsed)) body.value = parsed
    if (note.trim() !== '') body.note = note.trim()
    mutation.mutate(body)
  }

  const busy = mutation.isPending

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
          {t('detail.offer')}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="offer-status">{t('detail.offerStatus')}</Label>
          <Select value={status} onValueChange={setStatus} disabled={busy}>
            <SelectTrigger id="offer-status" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {leadStatuses.map((s) => (
                <SelectItem key={s.value} value={s.value}>
                  {s.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="offer-value">{t('detail.offerValue')}</Label>
          <Input
            id="offer-value"
            type="number"
            min={0}
            inputMode="decimal"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            disabled={busy}
            placeholder="0"
          />
        </div>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="offer-note">{t('detail.offerNote')}</Label>
          <Textarea
            id="offer-note"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            disabled={busy}
            rows={3}
            maxLength={2000}
          />
        </div>

        <Button onClick={onSave} disabled={busy} className="self-end">
          {busy ? t('common.saving') : t('common.save')}
        </Button>
      </CardContent>
    </Card>
  )
}
