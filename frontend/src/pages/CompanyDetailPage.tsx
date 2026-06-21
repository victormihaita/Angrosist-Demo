import { ArrowLeft } from 'lucide-react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useCompanyDetail } from '@/hooks/useDashboard'
import { t, formatRON } from '@/lib/strings'

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

export function CompanyDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const {
    data: company,
    isLoading,
    error,
    refetch,
  } = useCompanyDetail(id ?? '')

  if (isLoading) {
    return (
      <div className="max-w-7xl mx-auto px-4 py-8 w-full">
        <Skeleton className="h-8 w-64 mb-6" />
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <Skeleton className="h-64 w-full" />
          <div className="flex flex-col gap-6">
            <Skeleton className="h-40 w-full" />
            <Skeleton className="h-40 w-full" />
          </div>
        </div>
      </div>
    )
  }

  if (error || !company) {
    return (
      <div className="flex flex-col items-center gap-3 py-20">
        <p className="text-sm text-destructive">{t.companies.notFound}</p>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            {t.common.retry}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => navigate('/dashboard/companies')}
          >
            {t.common.back}
          </Button>
        </div>
      </div>
    )
  }

  const verification = company.verification
  const administrators = Array.isArray(verification?.administrators)
    ? verification.administrators
    : []
  const financials = company.financials ?? []

  return (
    <div className="max-w-7xl mx-auto px-4 py-8 w-full min-w-0">
      <div className="flex items-center gap-3 mb-6">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => navigate('/dashboard/companies')}
        >
          <ArrowLeft className="h-4 w-4 mr-1" />
          {t.common.back}
        </Button>
        <h2 className="text-lg font-semibold flex-1 truncate">
          {company.name || 'Companie'}
        </h2>
        {company.is_active != null && (
          <Badge variant={company.is_active ? 'default' : 'destructive'}>
            {company.is_active ? t.companies.active : t.companies.inactive}
          </Badge>
        )}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 items-start">
        {/* Identity */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
              {t.companies.identity}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="flex flex-col gap-3">
              <Field label={t.companies.colName} value={company.name} />
              <Field
                label={t.companies.colCui}
                value={company.cui || company.reg_no}
              />
              <Field label={t.companies.regNo} value={company.reg_no} />
              <Field label={t.companies.colCountry} value={company.country} />
              <Field label={t.companies.colCaen} value={company.caen} />
              <Field label={t.companies.colVat} value={company.vat_status} />
              <Field label={t.companies.address} value={company.address} />
              <Field label={t.companies.county} value={company.county} />
            </dl>

            {company.roles && company.roles.length > 0 && (
              <div className="mt-3">
                <dt className="text-xs text-muted-foreground mb-1.5">
                  {t.companies.colRoles}
                </dt>
                <div className="flex flex-wrap gap-1.5">
                  {company.roles.map((r) => (
                    <Badge key={r} variant="outline">
                      {r}
                    </Badge>
                  ))}
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        <div className="flex flex-col gap-6 min-w-0">
          {/* Verification */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                {t.companies.verification}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {verification ? (
                <dl className="flex flex-col gap-3">
                  <Field
                    label={t.companies.colVat}
                    value={verification.vat_status}
                  />
                  {administrators.length > 0 && (
                    <div>
                      <dt className="text-xs text-muted-foreground">
                        {t.detail.administrators}
                      </dt>
                      <dd className="text-sm font-medium mt-0.5">
                        {administrators.join(', ')}
                      </dd>
                    </div>
                  )}
                  <Field
                    label={t.detail.checkedAt}
                    value={
                      verification.checked_at
                        ? new Date(verification.checked_at).toLocaleString(
                            'ro-RO',
                          )
                        : undefined
                    }
                  />
                </dl>
              ) : (
                <p className="text-sm text-muted-foreground">
                  {t.detail.noVerification}
                </p>
              )}
            </CardContent>
          </Card>

          {/* Financials (only when present) */}
          {financials.length > 0 && (
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                  {t.companies.financials}
                </CardTitle>
              </CardHeader>
              <CardContent className="p-0">
                <Separator />
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t.companies.year}</TableHead>
                      <TableHead className="text-right">
                        {t.companies.turnover}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {financials.map((f) => (
                      <TableRow key={f.year}>
                        <TableCell className="tabular-nums">{f.year}</TableCell>
                        <TableCell className="text-right tabular-nums">
                          {formatRON(f.turnover)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}
