import React, { useRef } from 'react'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { SendHorizonal } from 'lucide-react'

interface Props {
  value: string
  onChange: (v: string) => void
  onSend: () => void
  disabled?: boolean
}

export function MessageInput({ value, onChange, onSend, disabled }: Props) {
  const ref = useRef<HTMLTextAreaElement>(null)

  function handleKey(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      if (!disabled && value.trim()) onSend()
    }
  }

  return (
    <div className="flex gap-2 px-4 py-3 border-t bg-background pb-[max(12px,env(safe-area-inset-bottom))]">
      <Textarea
        ref={ref}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={handleKey}
        placeholder="Scrieți mesajul... (Enter pentru trimitere)"
        disabled={disabled}
        rows={1}
        className="resize-none min-h-[40px] max-h-[120px] flex-1"
      />
      <Button
        onClick={onSend}
        disabled={disabled || !value.trim()}
        size="icon"
        className="shrink-0 self-end"
      >
        <SendHorizonal className="h-4 w-4" />
      </Button>
    </div>
  )
}
