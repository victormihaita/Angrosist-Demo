import { Languages } from 'lucide-react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useT } from '@/lib/i18n'
import type { Lang } from '@/lib/i18n'

/**
 * Compact RO/EN language switch for the app shell. Uses the shadcn Select
 * primitive as-is; switching updates the i18n context (persisted to
 * localStorage) with no reload. The short codes (RO/EN) keep the trigger tight
 * in the nav; full names appear in the dropdown for clarity + a11y.
 */
export function LanguageToggle() {
  const { lang, setLang, t } = useT()

  return (
    <Select value={lang} onValueChange={(v) => setLang(v as Lang)}>
      <SelectTrigger
        size="sm"
        className="w-[72px] gap-1.5"
        aria-label={t('lang.label')}
      >
        <Languages className="h-3.5 w-3.5 text-muted-foreground" />
        <SelectValue />
      </SelectTrigger>
      <SelectContent align="end">
        <SelectItem value="ro">{t('lang.ro')}</SelectItem>
        <SelectItem value="en">{t('lang.en')}</SelectItem>
      </SelectContent>
    </Select>
  )
}
