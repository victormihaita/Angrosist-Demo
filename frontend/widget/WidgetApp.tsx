import { useState, useRef } from 'react'
import { sendMessage, type ExtractedFields } from '../src/lib/api'

interface Message {
  role: 'user' | 'assistant'
  content: string
}

interface Props {
  apiUrl?: string
  onClose: () => void
}

const CONV_KEY = 'angrosist_widget_conv_id'

export function WidgetApp({ apiUrl, onClose }: Props) {
  const [messages, setMessages] = useState<Message[]>([
    {
      role: 'assistant',
      content: 'Bună ziua! Sunt asistentul Euro Intermed. Cu ce vă pot ajuta?',
    },
  ])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [_extracted, setExtracted] = useState<ExtractedFields>({})
  const convIdRef = useRef<string | null>(sessionStorage.getItem(CONV_KEY))
  const bottomRef = useRef<HTMLDivElement>(null)

  // Override API_URL if provided
  if (apiUrl) {
    ;(window as unknown as Record<string, unknown>).__ANGROSIST_API_URL__ = apiUrl
  }

  async function handleSend() {
    const text = input.trim()
    if (!text || loading) return
    setInput('')
    setMessages((p) => [...p, { role: 'user', content: text }])
    setLoading(true)
    try {
      const resp = await sendMessage(convIdRef.current, text)
      convIdRef.current = resp.conversation_id
      sessionStorage.setItem(CONV_KEY, resp.conversation_id)
      setExtracted(resp.extracted ?? {})
      setMessages((p) => [...p, { role: 'assistant', content: resp.reply }])
    } catch {
      setMessages((p) => [
        ...p,
        { role: 'assistant', content: 'Eroare de rețea. Încercați din nou.' },
      ])
    } finally {
      setLoading(false)
      setTimeout(() => bottomRef.current?.scrollIntoView({ behavior: 'smooth' }), 50)
    }
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
        {loading && (
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

      {/* Input */}
      <div style={{ padding: '10px 12px', borderTop: '1px solid #e5e7eb', display: 'flex', gap: '8px', flexShrink: 0 }}>
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend() } }}
          placeholder="Mesaj..."
          disabled={loading}
          style={{
            flex: 1, padding: '8px 12px', borderRadius: '8px',
            border: '1px solid #e5e7eb', outline: 'none', fontSize: '14px',
          }}
        />
        <button
          onClick={handleSend}
          disabled={loading || !input.trim()}
          style={{
            padding: '8px 14px', background: '#111827', color: '#fff',
            border: 'none', borderRadius: '8px', cursor: 'pointer',
            opacity: loading || !input.trim() ? 0.5 : 1,
          }}
        >
          ›
        </button>
      </div>
    </div>
  )
}
