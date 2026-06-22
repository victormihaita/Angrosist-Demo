import { useRef, useState, useCallback } from 'react'
import { uploadConversationPhoto, ApiError } from '@/lib/api'

/**
 * Inline-styled seller photo upload for the embeddable widget (PalletClearance
 * /sell flow). Deliberately shadcn-free to keep the widget bundle small and
 * isolated from host CSS — consistent with the rest of WidgetApp. Errors show
 * inline (no sonner in the widget). Disabled until a conversation id exists.
 */

type ItemStatus = 'uploading' | 'done' | 'error'

interface PhotoItem {
  localId: string
  previewUrl: string
  status: ItemStatus
}

interface Props {
  conversationId: string | null
  /** Per-conversation ownership token; echoed on upload or the backend 403s. */
  conversationToken: string | null
}

let counter = 0
function nextId(): string {
  counter += 1
  return `wphoto-${counter}-${Date.now()}`
}

export function WidgetSellerPhotos({ conversationId, conversationToken }: Props) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [items, setItems] = useState<PhotoItem[]>([])
  const [error, setError] = useState<string | null>(null)

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
      setError(null)

      Array.from(files).forEach((file) => {
        if (!file.type.startsWith('image/')) {
          setError('Doar imagini sunt permise.')
          return
        }
        const localId = nextId()
        const previewUrl = URL.createObjectURL(file)
        setItems((prev) => [...prev, { localId, previewUrl, status: 'uploading' }])

        void (async () => {
          try {
            await uploadConversationPhoto(conversationId, file, conversationToken)
            updateItem(localId, { status: 'done' })
          } catch (err) {
            const message =
              err instanceof ApiError
                ? err.message
                : 'Încărcarea a eșuat. Încercați din nou.'
            updateItem(localId, { status: 'error' })
            setError(message)
          }
        })()
      })
    },
    [conversationId, conversationToken, updateItem],
  )

  return (
    <div
      style={{
        padding: '8px 12px',
        borderTop: '1px solid #e5e7eb',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
        flexShrink: 0,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
        <button
          type="button"
          onClick={() => inputRef.current?.click()}
          disabled={disabled || busy}
          style={{
            padding: '6px 10px',
            background: '#fff',
            color: '#111827',
            border: '1px solid #d1d5db',
            borderRadius: '8px',
            cursor: disabled || busy ? 'not-allowed' : 'pointer',
            fontSize: '13px',
            opacity: disabled || busy ? 0.5 : 1,
          }}
        >
          {busy ? 'Se încarcă…' : '📷 Adaugă fotografii'}
        </button>
        {doneCount > 0 && (
          <span style={{ fontSize: '12px', color: '#6b7280' }} aria-live="polite">
            {doneCount === 1
              ? '1 fotografie încărcată'
              : `${doneCount} fotografii încărcate`}
          </span>
        )}
      </div>

      <input
        ref={inputRef}
        type="file"
        accept="image/*"
        multiple
        aria-label="Adaugă fotografii"
        style={{ display: 'none' }}
        onChange={(e) => {
          handleFiles(e.target.files)
          e.target.value = ''
        }}
      />

      {items.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '6px' }}>
          {items.map((it) => (
            <div
              key={it.localId}
              style={{
                position: 'relative',
                width: '44px',
                height: '44px',
                borderRadius: '6px',
                overflow: 'hidden',
                border: '1px solid #e5e7eb',
                background: '#f3f4f6',
              }}
            >
              <img
                src={it.previewUrl}
                alt=""
                loading="lazy"
                style={{ width: '100%', height: '100%', objectFit: 'cover' }}
              />
              <span
                style={{
                  position: 'absolute',
                  inset: 0,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  background: 'rgba(0,0,0,0.3)',
                  color: '#fff',
                  fontSize: '12px',
                }}
              >
                {it.status === 'uploading' ? '…' : it.status === 'done' ? '✓' : '!'}
              </span>
            </div>
          ))}
        </div>
      )}

      {error && (
        <span style={{ fontSize: '12px', color: '#dc2626' }}>{error}</span>
      )}
    </div>
  )
}
