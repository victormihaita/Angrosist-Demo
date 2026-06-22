import { useRef, useState, useCallback } from 'react'
import { ImagePlus, Loader2, Check, AlertCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { toast } from 'sonner'
import { uploadConversationPhoto, ApiError } from '@/lib/api'
import { useT } from '@/lib/i18n'

/**
 * Seller photo upload affordance for the PalletClearance/sell chat flow.
 *
 * Rendered ONLY when `intent === 'sell'` (the buyer/Angrosist flow never shows
 * it). The backend refuses to finalize a seller listing until ≥1 photo exists;
 * this control just makes uploading easy and shows what has been uploaded.
 *
 * It is disabled until a conversation id exists (i.e. after the first message),
 * because the upload endpoint targets a specific conversation. Each picked file
 * is uploaded independently via `uploadConversationPhoto`; per-file state and a
 * running count are shown, and server errors (409 cap / 400 non-image-oversize)
 * surface via sonner with the backend's exact message.
 */

type ItemStatus = 'uploading' | 'done' | 'error'

interface PhotoItem {
  /** Stable local key for React. */
  localId: string
  name: string
  /** Object URL for the local thumbnail (revoked on success swap / unmount). */
  previewUrl: string
  status: ItemStatus
  /** Remote URL once uploaded (currently unused for display, kept for parity). */
  remoteUrl?: string
}

interface Props {
  conversationId: string | null
  /** Per-conversation ownership token; echoed on upload or the backend 403s. */
  conversationToken: string | null
  /** Localized label for the trigger button. */
  label?: string
}

let counter = 0
function nextId(): string {
  counter += 1
  return `photo-${counter}-${Date.now()}`
}

export function SellerPhotoUpload({
  conversationId,
  conversationToken,
  label,
}: Props) {
  const { t } = useT()
  const inputRef = useRef<HTMLInputElement>(null)
  const [items, setItems] = useState<PhotoItem[]>([])
  const buttonLabel = label ?? t('chat.addPhotos')

  const doneCount = items.filter((i) => i.status === 'done').length
  const busy = items.some((i) => i.status === 'uploading')
  const disabled = !conversationId

  const updateItem = useCallback(
    (localId: string, patch: Partial<PhotoItem>) => {
      setItems((prev) =>
        prev.map((it) => (it.localId === localId ? { ...it, ...patch } : it)),
      )
    },
    [],
  )

  const handleFiles = useCallback(
    (files: FileList | null) => {
      if (!files || !conversationId) return

      Array.from(files).forEach((file) => {
        // Client-side guard mirrors the server (it is the real gate): images only.
        if (!file.type.startsWith('image/')) {
          toast.error(t('chat.photosOnlyImages'))
          return
        }

        const localId = nextId()
        const previewUrl = URL.createObjectURL(file)
        setItems((prev) => [
          ...prev,
          { localId, name: file.name, previewUrl, status: 'uploading' },
        ])

        void (async () => {
          try {
            const res = await uploadConversationPhoto(
              conversationId,
              file,
              conversationToken,
            )
            updateItem(localId, { status: 'done', remoteUrl: res.url })
          } catch (err) {
            const message =
              err instanceof ApiError ? err.message : t('chat.uploadFailed')
            updateItem(localId, { status: 'error' })
            toast.error(message)
          }
        })()
      })
    },
    [conversationId, conversationToken, updateItem, t],
  )

  return (
    <div className="flex flex-col gap-2 px-4 pt-3">
      <div className="flex items-center gap-3 flex-wrap">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled || busy}
          onClick={() => inputRef.current?.click()}
        >
          {busy ? (
            <Loader2 className="h-4 w-4 mr-1.5 animate-spin" />
          ) : (
            <ImagePlus className="h-4 w-4 mr-1.5" />
          )}
          {buttonLabel}
        </Button>

        {doneCount > 0 && (
          <span className="text-xs text-muted-foreground" aria-live="polite">
            {doneCount === 1
              ? t('chat.photoCountOne')
              : t('chat.photoCountOther', { n: doneCount })}
          </span>
        )}

        {disabled && (
          <span className="text-xs text-muted-foreground">
            {t('chat.photoHint')}
          </span>
        )}
      </div>

      <input
        ref={inputRef}
        type="file"
        accept="image/*"
        multiple
        className="hidden"
        aria-label={buttonLabel}
        onChange={(e) => {
          handleFiles(e.target.files)
          // Reset so picking the same file again re-triggers change.
          e.target.value = ''
        }}
      />

      {items.length > 0 && (
        <ul className="flex flex-wrap gap-2 list-none p-0 m-0">
          {items.map((it) => (
            <li
              key={it.localId}
              className="relative h-14 w-14 overflow-hidden rounded-md border bg-muted"
              title={it.name}
            >
              <img
                src={it.previewUrl}
                alt={it.name}
                loading="lazy"
                className="h-full w-full object-cover"
              />
              <span className="absolute inset-0 flex items-center justify-center bg-black/30">
                {it.status === 'uploading' && (
                  <Loader2 className="h-4 w-4 text-white animate-spin" />
                )}
                {it.status === 'done' && (
                  <Check className="h-4 w-4 text-white" />
                )}
                {it.status === 'error' && (
                  <AlertCircle className="h-4 w-4 text-white" />
                )}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
