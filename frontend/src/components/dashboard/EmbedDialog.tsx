import { useState } from 'react'
import { Code2, Copy, Check } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { useT } from '@/lib/i18n'

const API_URL = import.meta.env.VITE_API_URL ?? window.location.origin
const WIDGET_URL = `${window.location.origin}/widget.js`

const embedCode = `<!-- Euro Intermed Chat Widget -->
<script src="${WIDGET_URL}"></script>
<script>
  AngrosistChat.init({ apiUrl: '${API_URL}' });
</script>`

// Optional PalletClearance seller variant — vertical/intent default to
// angrosist/buy when omitted, so the snippet above keeps existing embeds working.
const sellerEmbedCode = `<!-- PalletClearance — flux vânzător (cu fotografii) -->
<script src="${WIDGET_URL}"></script>
<script>
  AngrosistChat.init({
    apiUrl: '${API_URL}',
    vertical: 'palletclearance',
    intent: 'sell'
  });
</script>`

export function EmbedDialog() {
  const { t } = useT()
  const [open, setOpen] = useState(false)
  const [copied, setCopied] = useState<'default' | 'seller' | null>(null)

  function copy(which: 'default' | 'seller') {
    navigator.clipboard.writeText(
      which === 'seller' ? sellerEmbedCode : embedCode,
    )
    setCopied(which)
    setTimeout(() => setCopied(null), 2000)
  }

  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
        <Code2 className="h-3.5 w-3.5 mr-1.5" />
        {t('embed.trigger')}
      </Button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="w-[calc(100vw-2rem)] max-w-xl rounded-xl">
          <DialogHeader>
            <DialogTitle>{t('embed.title')}</DialogTitle>
            <DialogDescription>{t('embed.description')}</DialogDescription>
          </DialogHeader>

          <div className="relative mt-2">
            <pre className="bg-muted rounded-lg p-4 pr-10 text-xs leading-relaxed whitespace-pre-wrap break-all">
              {embedCode}
            </pre>
            <Button
              variant="ghost"
              size="icon"
              className="absolute top-2 right-2 h-7 w-7"
              onClick={() => copy('default')}
              aria-label={copied === 'default' ? t('embed.copied') : t('embed.copy')}
            >
              {copied === 'default'
                ? <Check className="h-3.5 w-3.5 text-green-500" />
                : <Copy className="h-3.5 w-3.5" />}
            </Button>
          </div>

          <p className="text-xs text-muted-foreground mt-1">
            {t('embed.floatNote')}
          </p>

          <div className="mt-4">
            <p className="text-xs font-medium mb-1.5">
              {t('embed.sellerHeading')}
            </p>
            <div className="relative">
              <pre className="bg-muted rounded-lg p-4 pr-10 text-xs leading-relaxed whitespace-pre-wrap break-all">
                {sellerEmbedCode}
              </pre>
              <Button
                variant="ghost"
                size="icon"
                className="absolute top-2 right-2 h-7 w-7"
                onClick={() => copy('seller')}
                aria-label={copied === 'seller' ? t('embed.copied') : t('embed.copy')}
              >
                {copied === 'seller'
                  ? <Check className="h-3.5 w-3.5 text-green-500" />
                  : <Copy className="h-3.5 w-3.5" />}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground mt-1">
              <code>vertical</code> și <code>intent</code> sunt opționale —
              implicit <code>angrosist</code>/<code>buy</code>.
            </p>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
