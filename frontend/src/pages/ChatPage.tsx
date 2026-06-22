import { useState, useRef, useEffect, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { MessageList } from '@/components/chat/MessageList'
import { MessageInput } from '@/components/chat/MessageInput'
import { SellerPhotoUpload } from '@/components/chat/SellerPhotoUpload'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useChat } from '@/hooks/useChat'
import { useT } from '@/lib/i18n'
import type { ChatIntent, ChatVertical } from '@/lib/api'

const CONV_KEY = 'angrosist_conv_id'

/** Flow presets selectable in the UI / via ?vertical=…&intent=…. */
type FlowKey = 'angrosist-buy' | 'palletclearance-sell'

// Static, non-localized flow routing (vertical/intent are backend codes). The
// human-facing label + greeting bubble are localized at render via the i18n keys
// referenced here. NOTE: the agent's CONVERSATION language is server-driven and
// independent of this UI greeting/label.
const FLOW_META: Record<
  FlowKey,
  {
    vertical: ChatVertical
    intent: ChatIntent
    labelKey: 'chat.flowAngrosistBuy' | 'chat.flowPalletSell'
    greetingKey: 'chat.greetingAngrosist' | 'chat.greetingPallet'
  }
> = {
  'angrosist-buy': {
    vertical: 'angrosist',
    intent: 'buy',
    labelKey: 'chat.flowAngrosistBuy',
    greetingKey: 'chat.greetingAngrosist',
  },
  'palletclearance-sell': {
    vertical: 'palletclearance',
    intent: 'sell',
    labelKey: 'chat.flowPalletSell',
    greetingKey: 'chat.greetingPallet',
  },
}

function resolveFlowKey(params: URLSearchParams): FlowKey {
  if (
    params.get('vertical') === 'palletclearance' &&
    params.get('intent') === 'sell'
  ) {
    return 'palletclearance-sell'
  }
  return 'angrosist-buy'
}

/**
 * The chat surface for a single flow. Keyed by `flowKey` from the parent so
 * switching flows fully resets the chat engine (greeting, conversation id, SSE).
 */
function ChatSurface({ flowKey }: { flowKey: FlowKey }) {
  const { t } = useT()
  const flow = FLOW_META[flowKey]
  const { messages, typing, send, conversationId, conversationToken, intent } =
    useChat({
    // Separate storage per flow so switching doesn't resume the wrong conversation.
    convStorageKey: `${CONV_KEY}_${flowKey}`,
    greeting: t(flow.greetingKey),
    vertical: flow.vertical,
    intent: flow.intent,
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
    <>
      {/* Scrollable messages */}
      <div className="flex-1 overflow-y-auto">
        <div className="max-w-2xl mx-auto px-4">
          <MessageList messages={messages} loading={typing} />
          <div ref={bottomRef} />
        </div>
      </div>

      {/* Seller-only photo control: hidden for buyer/Angrosist flows */}
      {intent === 'sell' && (
        <div className="shrink-0 max-w-2xl w-full mx-auto">
          <SellerPhotoUpload
            conversationId={conversationId}
            conversationToken={conversationToken}
            label={t('chat.addPhotos')}
          />
        </div>
      )}

      {/* Input — stays above keyboard on mobile thanks to h-dvh on root */}
      <div className="shrink-0 max-w-2xl w-full mx-auto">
        <MessageInput
          value={input}
          onChange={setInput}
          onSend={handleSend}
          disabled={typing}
          placeholder={t('chat.inputPlaceholder')}
          sendLabel={t('chat.send')}
        />
      </div>
    </>
  )
}

export function ChatPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const { t } = useT()
  const initialFlow = useMemo(() => resolveFlowKey(searchParams), [searchParams])
  const [flowKey, setFlowKey] = useState<FlowKey>(initialFlow)

  function handleFlowChange(next: string) {
    const key = next as FlowKey
    setFlowKey(key)
    // Reflect the choice in the URL so the flow is shareable/refresh-stable.
    const sp = new URLSearchParams(searchParams)
    sp.set('vertical', FLOW_META[key].vertical)
    sp.set('intent', FLOW_META[key].intent)
    setSearchParams(sp, { replace: true })
  }

  return (
    // h-full fills the flex-1 overflow-hidden main from App
    <div className="flex flex-col h-full">
      {/* Flow selector */}
      <div className="shrink-0 max-w-2xl w-full mx-auto px-4 pt-3">
        <Select value={flowKey} onValueChange={handleFlowChange}>
          <SelectTrigger
            className="w-full sm:w-72"
            aria-label={t('chat.flowSelector')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(Object.keys(FLOW_META) as FlowKey[]).map((k) => (
              <SelectItem key={k} value={k}>
                {t(FLOW_META[k].labelKey)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Remount on flow change to reset the chat engine cleanly. */}
      <ChatSurface key={flowKey} flowKey={flowKey} />
    </div>
  )
}
