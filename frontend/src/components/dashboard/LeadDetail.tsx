import { ArrowLeft } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { MessageList } from '@/components/chat/MessageList'
import type { LeadDetail as LeadDetailType } from '@/lib/api'

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

// Model messages store text inside a base64-encoded JSON array of Gemini parts.
// Extract all string parts (text) when the plain content field is empty.
function resolveContent(m: import('@/lib/api').TranscriptMessage): string {
  if (m.content) return m.content
  if (m.role === 'model' && m.tool_calls) {
    try {
      const bytes = Uint8Array.from(atob(m.tool_calls), c => c.charCodeAt(0))
      const json = new TextDecoder('utf-8').decode(bytes)
      const parts: unknown[] = JSON.parse(json)
      return parts.filter((p): p is string => typeof p === 'string').join('')
    } catch {
      return ''
    }
  }
  return ''
}

interface Field { label: string; value?: string | number | null }

interface Props {
  lead: LeadDetailType
}

export function LeadDetail({ lead }: Props) {
  const navigate = useNavigate()

  const fields: Field[] = [
    { label: 'Companie', value: lead.company_name },
    { label: 'CUI', value: lead.cui },
    { label: 'Adresă', value: lead.address },
    { label: 'Județ', value: lead.county },
    { label: 'Produs', value: lead.product_name },
    { label: 'Cantitate', value: lead.quantity != null ? `${lead.quantity} ${lead.unit}` : undefined },
    { label: 'Locație livrare', value: lead.delivery_location },
    { label: 'Telefon', value: lead.phone },
    { label: 'Email', value: lead.email },
  ]

  // Map transcript to chat Message format (skip tool messages)
  const chatMessages = (lead.transcript ?? [])
    .filter((m) => m.role === 'user' || m.role === 'model')
    .map((m) => ({
      role: (m.role === 'model' ? 'assistant' : 'user') as 'user' | 'assistant',
      content: resolveContent(m),
    }))
    .filter((m) => m.content)

  return (
    <div>
      <div className="flex items-center gap-3 mb-6">
        <Button variant="ghost" size="sm" onClick={() => navigate('/dashboard')}>
          <ArrowLeft className="h-4 w-4 mr-1" />
          Înapoi
        </Button>
        <h2 className="text-lg font-semibold flex-1">{lead.company_name || 'Lead'}</h2>
        <Badge variant={STATUS_VARIANT[lead.status] ?? 'secondary'}>
          {STATUS_LABEL[lead.status] ?? lead.status}
        </Badge>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 items-stretch">
        {/* Left: extracted fields */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
              Detalii Lead
            </CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="flex flex-col gap-3">
              {fields.map((f) =>
                f.value ? (
                  <div key={f.label}>
                    <dt className="text-xs text-muted-foreground">{f.label}</dt>
                    <dd className="text-sm font-medium mt-0.5">{String(f.value)}</dd>
                  </div>
                ) : null,
              )}
            </dl>
          </CardContent>
        </Card>

        {/* Right: transcript — capped to viewport, scrolls internally */}
        <Card className="flex flex-col">
          <CardHeader className="pb-3 shrink-0">
            <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
              Transcriere conversație
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0 overflow-y-auto max-h-[calc(100vh-320px)]">
            {chatMessages.length > 0 ? (
              <MessageList messages={chatMessages} readonly />
            ) : (
              <p className="text-sm text-muted-foreground px-4 py-3">Fără mesaje.</p>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
