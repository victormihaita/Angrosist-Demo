import { useState, useRef, useEffect } from 'react'
import { MessageList } from '@/components/chat/MessageList'
import { MessageInput } from '@/components/chat/MessageInput'
import { useChat } from '@/hooks/useChat'

const CONV_KEY = 'angrosist_conv_id'

export function ChatPage() {
  const { messages, typing, send } = useChat({
    convStorageKey: CONV_KEY,
    greeting:
      'Bună ziua! Sunt asistentul Euro Intermed pentru achiziții en-gros. Cu ce vă pot ajuta astăzi?',
  })
  const [input, setInput] = useState('')
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, typing])

  function handleSend() {
    if (!input.trim() || typing) return
    send(input)
    setInput('')
  }

  return (
    // h-full fills the flex-1 overflow-hidden main from App
    <div className="flex flex-col h-full">
      {/* Scrollable messages */}
      <div className="flex-1 overflow-y-auto">
        <div className="max-w-2xl mx-auto px-4">
          <MessageList messages={messages} loading={typing} />
          <div ref={bottomRef} />
        </div>
      </div>

      {/* Input — stays above keyboard on mobile thanks to h-dvh on root */}
      <div className="shrink-0 max-w-2xl w-full mx-auto">
        <MessageInput
          value={input}
          onChange={setInput}
          onSend={handleSend}
          disabled={typing}
        />
      </div>
    </div>
  )
}
