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
import { administratorName } from '@/lib/api'
import { useCompanyDetail } from '@/hooks/useDashboard'
import {
  useT,
  useEnums,
  formatRON,
  formatDate,
  formatDateTime,
} from '@/lib/i18n'

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

/** shadcn Badge variant per canonical VAT status (no custom CSS). */
function vatBadgeVariant(
  status: string | null | undefined,
): 'default' | 'destructive' | 'secondary' | 'outline' {
  switch (status) {
    case 'active':
      return 'default'
    case 'inactive':
      return 'destructive'
    case 'not_registered':
      return 'secondary'
    default:
      return 'outline'
  }
}

export function CompanyDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { t, lang } = useT()
  const { roleLabel, vatLabel } = useEnums()
  const { data: company, isLoading, error, refetch } = useCompanyDetail(id ?? '')

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
        <p className="text-sm text-destructive">{t('companies.notFound')}</p>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            {t('common.retry')}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => navigate('/dashboard/companies')}
          >
            {t('common.back')}
          </Button>
        </div>
      </div>
    )
  }

  const verification = company.verification
  // Administrators may live on the detail body or the verification snapshot, as
  // string[] or object[]. Prefer the detail field; fall back to verification.
  const rawAdmins =
    (company.administrators?.length
      ? company.administrators
      : verification?.administrators) ?? []
  const administrators = rawAdmins
    .map((a) => administratorName(a))
    .filter((name) => name.length > 0)
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
          {t('common.back')}
        </Button>
        <h2 className="text-lg font-semibold flex-1 truncate">
          {company.name || t('companies.fallbackTitle')}
        </h2>
        {company.is_active != null && (
          <Badge variant={company.is_active ? 'default' : 'destructive'}>
            {company.is_active
              ? t('companies.active')
              : t('companies.inactive')}
          </Badge>
        )}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 items-start">
        {/* Identity / registry */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
              {t('companies.identity')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="flex flex-col gap-3">
              <Field label={t('companies.colName')} value={company.name} />
              <Field
                label={t('companies.colCui')}
                value={company.cui || company.reg_no}
              />
              <Field label={t('companies.colCountry')} value={company.country} />
              <Field label={t('companies.regNo')} value={company.reg_no} />
              <Field
                label={t('companies.registrationNumber')}
                value={company.registration_number}
              />
              <Field
                label={t('companies.registrationDate')}
                value={
                  company.registration_date
                    ? formatDate(lang, company.registration_date)
                    : undefined
                }
              />
              <Field
                label={t('companies.legalForm')}
                value={company.legal_form}
              />

              {/* VAT status — localized label + colored badge (fixes bare "unknown"). */}
              <div>
                <dt className="text-xs text-muted-foreground mb-1.5">
                  {t('companies.vatStatus')}
                </dt>
                <dd>
                  <Badge variant={vatBadgeVariant(company.vat_status)}>
                    {vatLabel(company.vat_status)}
                  </Badge>
                </dd>
              </div>

              {/* e-Factura yes/no badge. */}
              {company.e_factura != null && (
                <div>
                  <dt className="text-xs text-muted-foreground mb-1.5">
                    {t('companies.eFactura')}
                  </dt>
                  <dd>
                    <Badge
                      variant={company.e_factura ? 'default' : 'secondary'}
                    >
                      {company.e_factura ? t('detail.yes') : t('detail.no')}
                    </Badge>
                  </dd>
                </div>
              )}

              <Field label={t('companies.colCaen')} value={company.caen} />
              <Field label={t('companies.address')} value={company.address} />
              <Field label={t('companies.county')} value={company.county} />
            </dl>

            {company.roles && company.roles.length > 0 && (
              <div className="mt-3">
                <dt className="text-xs text-muted-foreground mb-1.5">
                  {t('companies.colRoles')}
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
          </CardContent>
        </Card>

        <div className="flex flex-col gap-6 min-w-0">
          {/* Administrators */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                {t('companies.administrators')}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {administrators.length > 0 ? (
                <ul className="flex flex-col gap-1.5">
                  {administrators.map((name, i) => (
                    <li key={`${name}-${i}`} className="text-sm font-medium">
                      {name}
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-sm text-muted-foreground">
                  {t('common.none')}
                </p>
              )}
            </CardContent>
          </Card>

          {/* Verification */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                {t('companies.verification')}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {verification ? (
                <dl className="flex flex-col gap-3">
                  <div>
                    <dt className="text-xs text-muted-foreground mb-1.5">
                      {t('companies.vatStatus')}
                    </dt>
                    <dd>
                      <Badge variant={vatBadgeVariant(verification.vat_status)}>
                        {vatLabel(verification.vat_status)}
                      </Badge>
                    </dd>
                  </div>
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

          {/* Financials (multi-year) */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                {t('companies.financials')}
              </CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              {financials.length > 0 ? (
                <>
                  <Separator />
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('companies.year')}</TableHead>
                        <TableHead className="text-right">
                          {t('companies.turnover')}
                        </TableHead>
                        <TableHead className="text-right">
                          {t('companies.netProfit')}
                        </TableHead>
                        <TableHead className="text-right">
                          {t('companies.employees')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {financials.map((f) => (
                        <TableRow key={f.year}>
                          <TableCell className="tabular-nums">
                            {f.year}
                          </TableCell>
                          <TableCell className="text-right tabular-nums">
                            {formatRON(lang, f.turnover, t('common.none'))}
                          </TableCell>
                          <TableCell className="text-right tabular-nums">
                            {formatRON(lang, f.net_profit, t('common.none'))}
                          </TableCell>
                          <TableCell className="text-right tabular-nums">
                            {f.employees == null
                              ? t('common.none')
                              : f.employees}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </>
              ) : (
                <div className="px-6 pb-6">
                  <p className="text-sm text-muted-foreground">
                    {t('companies.noFinancials')}
                  </p>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}
