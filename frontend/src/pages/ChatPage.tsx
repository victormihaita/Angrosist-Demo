import { useState, useRef, useEffect } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { MessageList, type Message } from '@/components/chat/MessageList'
import { MessageInput } from '@/components/chat/MessageInput'
import { sendMessage } from '@/lib/api'

const CONV_KEY = 'angrosist_conv_id'

export function ChatPage() {
  const [messages, setMessages] = useState<Message[]>([
    {
      role: 'assistant',
      content:
        'Bună ziua! Sunt asistentul Euro Intermed pentru achiziții en-gros. Cu ce vă pot ajuta astăzi?',
    },
  ])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const convIdRef = useRef<string | null>(sessionStorage.getItem(CONV_KEY))

  useEffect(() => {
    const apiUrl = import.meta.env.VITE_API_URL ?? ''
    import('../../widget/widget-entry').then(() => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(window as any).AngrosistChat?.init({ apiUrl })
    })
  }, [])

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
    <div className="flex flex-col h-screen bg-background">
      {/* Header */}
      <div className="border-b px-6 py-4 flex items-center justify-between shrink-0">
        <div>
          <h1 className="font-semibold text-base">Angrosist — Euro Intermed</h1>
          <p className="text-xs text-muted-foreground">Platformă B2B de achiziții en-gros</p>
        </div>
        <a href="/dashboard" className="text-xs text-muted-foreground hover:text-foreground transition-colors">
          Dashboard →
        </a>
      </div>

      {/* Chat area */}
      <div className="flex flex-1 overflow-hidden justify-center">
        <div className="flex flex-col w-full max-w-2xl">
          <ScrollArea className="flex-1">
            <MessageList messages={messages} loading={loading} />
          </ScrollArea>
          <MessageInput
            value={input}
            onChange={setInput}
            onSend={handleSend}
            disabled={loading}
          />
        </div>
      </div>
    </div>
  )
}
