import { useState, useRef, useEffect } from 'react'
import { useChat } from '@/hooks/useChat'
import type { ChatIntent, ChatVertical } from '@/lib/api'
import { WidgetSellerPhotos } from './WidgetSellerPhotos'

interface Props {
  apiUrl?: string
  vertical?: ChatVertical
  intent?: ChatIntent
  onClose: () => void
}

const CONV_KEY = 'angrosist_widget_conv_id'

// `widget-entry.tsx` already stashes `config.apiUrl` on
// `window.__ANGROSIST_API_URL__` at module init (before this component renders),
// so getApiBase() resolves correctly on the first send. The effect below keeps
// the global in sync if the prop changes.
export function WidgetApp({ apiUrl, vertical, intent, onClose }: Props) {
  const {
    messages,
    typing,
    extracted: _extracted,
    send,
    conversationId,
    conversationToken,
    intent: resolvedIntent,
  } = useChat({
    // Per-flow storage so a buyer and a seller session don't collide on the host.
    convStorageKey: `${CONV_KEY}_${vertical ?? 'angrosist'}_${intent ?? 'buy'}`,
    greeting:
      intent === 'sell'
        ? 'Bună ziua! Sunt asistentul PalletClearance. Descrieți lotul și adăugați fotografii.'
        : 'Bună ziua! Sunt asistentul Euro Intermed. Cu ce vă pot ajuta?',
    errorMessage: 'Eroare de rețea. Încercați din nou.',
    vertical,
    intent,
  })
  const [input, setInput] = useState('')
  const bottomRef = useRef<HTMLDivElement>(null)

  // Keep the global override in sync with the prop (side effect, out of render).
  useEffect(() => {
    if (apiUrl) {
      ;(window as unknown as Record<string, unknown>).__ANGROSIST_API_URL__ = apiUrl
    }
  }, [apiUrl])

  // Auto-scroll on new messages / typing changes.
  useEffect(() => {
    const t = setTimeout(
      () => bottomRef.current?.scrollIntoView({ behavior: 'smooth' }),
      50,
    )
    return () => clearTimeout(t)
  }, [messages, typing])

  function handleSend() {
    if (!input.trim() || typing) return
    send(input)
    setInput('')
  }

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        width: '360px',
        height: '500px',
        borderRadius: '16px',
        boxShadow: '0 8px 32px rgba(0,0,0,0.18)',
        background: '#fff',
        fontFamily: 'system-ui, sans-serif',
        fontSize: '14px',
        overflow: 'hidden',
        border: '1px solid #e5e7eb',
      }}
    >
      {/* Header */}
      <div
        style={{
          padding: '12px 16px',
          background: '#111827',
          color: '#fff',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexShrink: 0,
        }}
      >
        <span style={{ fontWeight: 600 }}>Euro Intermed</span>
        <button
          onClick={onClose}
          style={{ background: 'none', border: 'none', color: '#9ca3af', cursor: 'pointer', fontSize: '18px' }}
        >
          ×
        </button>
      </div>

      {/* Messages */}
      <div style={{ flex: 1, overflowY: 'auto', padding: '12px', display: 'flex', flexDirection: 'column', gap: '8px' }}>
        {messages.map((m, i) => (
          <div key={i} style={{ display: 'flex', justifyContent: m.role === 'user' ? 'flex-end' : 'flex-start' }}>
            <div
              style={{
                maxWidth: '80%',
                padding: '8px 12px',
                borderRadius: m.role === 'user' ? '16px 16px 4px 16px' : '16px 16px 16px 4px',
                background: m.role === 'user' ? '#111827' : '#f3f4f6',
                color: m.role === 'user' ? '#fff' : '#111827',
                lineHeight: 1.5,
              }}
            >
              {m.content}
            </div>
          </div>
        ))}
        {typing && (
          <div style={{ display: 'flex', gap: '4px', padding: '8px 0' }}>
            {[0, 1, 2].map((i) => (
              <div
                key={i}
                style={{
                  width: '8px', height: '8px', borderRadius: '50%', background: '#9ca3af',
                  animation: 'bounce 1s infinite',
                  animationDelay: `${i * 150}ms`,
                }}
              />
            ))}
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Seller-only photo control (PalletClearance/sell). Hidden otherwise. */}
      {resolvedIntent === 'sell' && (
        <WidgetSellerPhotos
          conversationId={conversationId}
          conversationToken={conversationToken}
        />
      )}

      {/* Input */}
      <div style={{ padding: '10px 12px', borderTop: '1px solid #e5e7eb', display: 'flex', gap: '8px', flexShrink: 0 }}>
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend() } }}
          placeholder="Mesaj..."
          disabled={typing}
          style={{
            flex: 1, padding: '8px 12px', borderRadius: '8px',
            border: '1px solid #e5e7eb', outline: 'none', fontSize: '14px',
          }}
        />
        <button
          onClick={handleSend}
          disabled={typing || !input.trim()}
          style={{
            padding: '8px 14px', background: '#111827', color: '#fff',
            border: 'none', borderRadius: '8px', cursor: 'pointer',
            opacity: typing || !input.trim() ? 0.5 : 1,
          }}
        >
          ›
        </button>
      </div>
    </div>
  )
}
