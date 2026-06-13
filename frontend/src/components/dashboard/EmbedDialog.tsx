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

const API_URL = import.meta.env.VITE_API_URL ?? window.location.origin
const WIDGET_URL = `${window.location.origin}/widget.js`

const embedCode = `<!-- Euro Intermed Chat Widget -->
<script src="${WIDGET_URL}"></script>
<script>
  AngrosistChat.init({ apiUrl: '${API_URL}' });
</script>`

export function EmbedDialog() {
  const [open, setOpen] = useState(false)
  const [copied, setCopied] = useState(false)

  function copy() {
    navigator.clipboard.writeText(embedCode)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
        <Code2 className="h-3.5 w-3.5 mr-1.5" />
        Embed Widget
      </Button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="w-[calc(100vw-2rem)] max-w-xl rounded-xl">
          <DialogHeader>
            <DialogTitle>Embed Chat Widget</DialogTitle>
            <DialogDescription>
              Adaugă codul de mai jos pe orice pagină pentru a activa chat-ul Euro Intermed.
            </DialogDescription>
          </DialogHeader>

          <div className="relative mt-2">
            <pre className="bg-muted rounded-lg p-4 pr-10 text-xs leading-relaxed whitespace-pre-wrap break-all">
              {embedCode}
            </pre>
            <Button
              variant="ghost"
              size="icon"
              className="absolute top-2 right-2 h-7 w-7"
              onClick={copy}
            >
              {copied
                ? <Check className="h-3.5 w-3.5 text-green-500" />
                : <Copy className="h-3.5 w-3.5" />}
            </Button>
          </div>

          <p className="text-xs text-muted-foreground mt-1">
            Widgetul apare ca un buton de chat în colțul din dreapta-jos al paginii.
          </p>
        </DialogContent>
      </Dialog>
    </>
  )
}
