import { useState, useRef, useEffect } from 'react'
import { MessageList, type Message } from '@/components/chat/MessageList'
import { MessageInput } from '@/components/chat/MessageInput'
import { sendMessage } from '@/lib/api'

const CONV_KEY = 'angrosist_conv_id'

export function ChatPage() {
  const [messages, setMessages] = useState<Message[]>([
    {
      role: 'assistant',
      content: 'Bună ziua! Sunt asistentul Euro Intermed pentru achiziții en-gros. Cu ce vă pot ajuta astăzi?',
    },
  ])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const convIdRef = useRef<string | null>(sessionStorage.getItem(CONV_KEY))
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, loading])

  async function handleSend() {
    const text = input.trim()
    if (!text || loading) return

    setInput('')
    setMessages((prev) => [...prev, { role: 'user', content: text }])
    setLoading(true)

    try {
      const resp = await sendMessage(convIdRef.current, text)
      convIdRef.current = resp.conversation_id
      sessionStorage.setItem(CONV_KEY, resp.conversation_id)
      setMessages((prev) => [...prev, { role: 'assistant', content: resp.reply }])
    } catch {
      setMessages((prev) => [
        ...prev,
        { role: 'assistant', content: 'A apărut o eroare. Vă rugăm încercați din nou.' },
      ])
    } finally {
      setLoading(false)
    }
  }

  return (
    // h-full fills the flex-1 overflow-hidden main from App
    <div className="flex flex-col h-full">
      {/* Scrollable messages */}
      <div className="flex-1 overflow-y-auto">
        <div className="max-w-2xl mx-auto px-4">
          <MessageList messages={messages} loading={loading} />
          <div ref={bottomRef} />
        </div>
      </div>

      {/* Input — stays above keyboard on mobile thanks to h-dvh on root */}
      <div className="shrink-0 max-w-2xl w-full mx-auto">
        <MessageInput
          value={input}
          onChange={setInput}
          onSend={handleSend}
          disabled={loading}
        />
      </div>
    </div>
  )
}
