import { ArrowLeft } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { MessageList, type Message } from '@/components/chat/MessageList'
import { StatusBadge } from '@/components/dashboard/StatusBadge'
import { OfferCard } from '@/components/dashboard/OfferCard'
import { AssigneeCard } from '@/components/dashboard/AssigneeCard'
import { useT, useEnums, formatDateTime } from '@/lib/i18n'
import type {
  AuthedLeadDetail,
  PublicUser,
  TranscriptMessage,
} from '@/lib/api'

// Model messages may store text inside a base64-encoded JSON array of parts;
// extract plain text when the content field is empty (matches the demo logic).
function resolveContent(m: TranscriptMessage): string {
  if (m.content) return m.content
  if ((m.role === 'model' || m.role === 'assistant') && m.tool_calls) {
    try {
      const bytes = Uint8Array.from(atob(m.tool_calls), (c) => c.charCodeAt(0))
      const json = new TextDecoder('utf-8').decode(bytes)
      const parts: unknown[] = JSON.parse(json)
      return parts.filter((p): p is string => typeof p === 'string').join('')
    } catch {
      return ''
    }
  }
  return ''
}

interface FieldProps {
  label: string
  value?: string | number | null
}

function Field({ label, value }: FieldProps) {
  if (value == null || value === '') return null
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm font-medium mt-0.5 break-words">{String(value)}</dd>
    </div>
  )
}

interface Props {
  lead: AuthedLeadDetail
  users: PublicUser[]
}

export function LeadDetail({ lead, users }: Props) {
  const navigate = useNavigate()
  const { t, lang } = useT()
  const { verticalLabel, roleLabel } = useEnums()

  const chatMessages: Message[] = (lead.transcript ?? [])
    .filter((m) => m.role === 'user' || m.role === 'model' || m.role === 'assistant')
    .map((m) => ({
      role: (m.role === 'user' ? 'user' : 'assistant') as 'user' | 'assistant',
      content: resolveContent(m),
    }))
    .filter((m) => m.content)

  const company = lead.company
  const contact = lead.contact
  const verification = company?.verification
  const sr = lead.sourcing_request

  const administrators = (() => {
    const a = verification?.administrators
    if (!a) return []
    if (Array.isArray(a)) return a
    return []
  })()

  return (
    <div>
      <div className="flex items-center gap-3 mb-6">
        <Button variant="ghost" size="sm" onClick={() => navigate('/dashboard')}>
          <ArrowLeft className="h-4 w-4 mr-1" />
          {t('common.back')}
        </Button>
        <h2 className="text-lg font-semibold flex-1 truncate">
          {lead.company_name || t('detail.fallbackTitle')}
        </h2>
        {lead.vertical && (
          <Badge variant="secondary">{verticalLabel(lead.vertical)}</Badge>
        )}
        <StatusBadge status={lead.status} />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 items-start">
        {/* Left column: extracted fields + transcript */}
        <div className="flex flex-col gap-6 min-w-0">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                {t('detail.extracted')}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <dl className="flex flex-col gap-3">
                <Field
                  label={t('pipeline.colProduct')}
                  value={lead.product_name}
                />
                <Field
                  label={t('pipeline.colQuantity')}
                  value={
                    lead.quantity != null
                      ? `${lead.quantity} ${lead.unit}`
                      : undefined
                  }
                />
                <Field
                  label={t('pipeline.colLocation')}
                  value={lead.delivery_location}
                />
                {sr?.budget != null && (
                  <Field label={t('detail.budget')} value={sr.budget} />
                )}
                {sr?.recurring && (
                  <Field label={t('detail.recurring')} value={t('detail.yes')} />
                )}
              </dl>
            </CardContent>
          </Card>

          <Card className="flex flex-col">
            <CardHeader className="pb-3 shrink-0">
              <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                {t('detail.transcript')}
              </CardTitle>
            </CardHeader>
            <CardContent className="p-0 overflow-y-auto max-h-[60vh]">
              {chatMessages.length > 0 ? (
                <MessageList messages={chatMessages} readonly />
              ) : (
                <p className="text-sm text-muted-foreground px-4 py-3">
                  {t('detail.noMessages')}
                </p>
              )}
            </CardContent>
          </Card>
        </div>

        {/* Right column: company, contact, offer, assignee */}
        <div className="flex flex-col gap-6 min-w-0">
          {company && (
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                  {t('detail.company')}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <dl className="flex flex-col gap-3">
                  <Field label={t('detail.company')} value={company.name} />
                  <Field
                    label={t('companies.colCui')}
                    value={company.cui || company.reg_no}
                  />
                  <Field label={t('detail.country')} value={company.country} />
                  <Field label={t('detail.caen')} value={company.caen} />
                  <Field
                    label={t('detail.vatStatus')}
                    value={company.vat_status || verification?.vat_status}
                  />
                </dl>

                {company.roles && company.roles.length > 0 && (
                  <div className="mt-3">
                    <dt className="text-xs text-muted-foreground mb-1.5">
                      {t('detail.roles')}
                    </dt>
                    <div className="flex flex-wrap gap-1.5">
                      {company.roles.map((r) => (
                        <Badge key={r} variant="outline">
                          {roleLabel(r)}
                        </Badge>
                      ))}
                    </div>
                  </div>
                )}

                <Separator className="my-4" />

                {verification ? (
                  <dl className="flex flex-col gap-3">
                    {administrators.length > 0 && (
                      <div>
                        <dt className="text-xs text-muted-foreground">
                          {t('detail.administrators')}
                        </dt>
                        <dd className="text-sm font-medium mt-0.5">
                          {administrators.join(', ')}
                        </dd>
                      </div>
                    )}
                    <Field
                      label={t('detail.checkedAt')}
                      value={
                        verification.checked_at
                          ? formatDateTime(lang, verification.checked_at)
                          : undefined
                      }
                    />
                  </dl>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    {t('detail.noVerification')}
                  </p>
                )}
              </CardContent>
            </Card>
          )}

          {(contact || lead.phone || lead.email) && (
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                  {t('detail.contact')}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <dl className="flex flex-col gap-3">
                  <Field label={t('detail.contactName')} value={contact?.name} />
                  <Field
                    label={t('detail.contactPhone')}
                    value={contact?.phone || lead.phone}
                  />
                  <Field
                    label={t('detail.contactEmail')}
                    value={contact?.email || lead.email}
                  />
                </dl>
              </CardContent>
            </Card>
          )}

          <OfferCard lead={lead} />
          <AssigneeCard lead={lead} users={users} />
        </div>
      </div>
    </div>
  )
}
