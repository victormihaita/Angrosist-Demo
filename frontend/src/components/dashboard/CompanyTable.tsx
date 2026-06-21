import { useNavigate } from 'react-router-dom'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { t } from '@/lib/strings'
import type { CompanySummary } from '@/lib/api'

interface Props {
  companies: CompanySummary[]
}

export function CompanyTable({ companies }: Props) {
  const navigate = useNavigate()

  return (
    <div className="rounded-lg border overflow-hidden">
      <div className="overflow-x-auto">
        <Table className="min-w-[820px]">
          <TableHeader>
            <TableRow>
              <TableHead>{t.companies.colName}</TableHead>
              <TableHead>{t.companies.colCui}</TableHead>
              <TableHead>{t.companies.colCountry}</TableHead>
              <TableHead>{t.companies.colCaen}</TableHead>
              <TableHead>{t.companies.colVat}</TableHead>
              <TableHead>{t.companies.colRoles}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {companies.map((c) => (
              <TableRow
                key={c.id}
                className="cursor-pointer"
                onClick={() => navigate(`/dashboard/companies/${c.id}`)}
              >
                <TableCell className="font-medium">
                  {c.name || t.common.none}
                </TableCell>
                <TableCell className="whitespace-nowrap tabular-nums">
                  {c.cui || c.reg_no || t.common.none}
                </TableCell>
                <TableCell>{c.country || t.common.none}</TableCell>
                <TableCell>{c.caen || t.common.none}</TableCell>
                <TableCell>{c.vat_status || t.common.none}</TableCell>
                <TableCell>
                  {c.roles && c.roles.length > 0 ? (
                    <div className="flex flex-wrap gap-1">
                      {c.roles.map((r) => (
                        <Badge key={r} variant="outline">
                          {r}
                        </Badge>
                      ))}
                    </div>
                  ) : (
                    <span className="text-muted-foreground">{t.common.none}</span>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}
