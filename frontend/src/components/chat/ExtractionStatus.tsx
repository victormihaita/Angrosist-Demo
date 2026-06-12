import { CheckCircle2, Circle } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { ExtractedFields } from '@/lib/api'

const FIELDS: { key: keyof ExtractedFields; label: string }[] = [
  { key: 'product_name', label: 'Produs' },
  { key: 'quantity', label: 'Cantitate' },
  { key: 'unit', label: 'Unitate' },
  { key: 'delivery_location', label: 'Locație' },
  { key: 'cui', label: 'CUI' },
  { key: 'phone', label: 'Telefon' },
  { key: 'email', label: 'Email' },
]

interface Props {
  extracted: ExtractedFields
}

export function ExtractionStatus({ extracted }: Props) {
  const filled = FIELDS.filter((f) => extracted[f.key] != null && extracted[f.key] !== '')
  if (filled.length === 0) return null

  return (
    <Card className="mx-4 mb-3 text-sm">
      <CardHeader className="py-2 px-4">
        <CardTitle className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
          Informații colectate
        </CardTitle>
      </CardHeader>
      <CardContent className="py-2 px-4">
        <ul className="flex flex-col gap-1">
          {FIELDS.map((f) => {
            const val = extracted[f.key]
            const done = val != null && val !== ''
            return (
              <li key={f.key} className="flex items-center gap-2">
                {done ? (
                  <CheckCircle2 className="h-3.5 w-3.5 text-green-500 shrink-0" />
                ) : (
                  <Circle className="h-3.5 w-3.5 text-muted-foreground/40 shrink-0" />
                )}
                <span className={done ? 'text-foreground' : 'text-muted-foreground'}>
                  {f.label}
                  {done && (
                    <span className="ml-1 text-muted-foreground">
                      — {String(val)}
                    </span>
                  )}
                </span>
              </li>
            )
          })}
        </ul>
      </CardContent>
    </Card>
  )
}
