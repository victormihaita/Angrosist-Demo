import { useCallback, useEffect, useRef, useState } from 'react'
import {
  sendMessage,
  subscribeToConversation,
  type ChatIntent,
  type ChatVertical,
  type ExtractedFields,
  type StreamMessage,
} from '@/lib/api'

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

interface UseChatOptions {
  /** sessionStorage key used to persist the conversation id across reloads. */
  convStorageKey: string
  /** The first assistant greeting bubble. */
  greeting: string
  /** Localized fallback message shown when both SSE and POST fail. */
  errorMessage?: string
  /** Flow vertical — sent on the first message only. Defaults to angrosist. */
  vertical?: ChatVertical
  /** Flow intent — sent on the first message only. Defaults to buy. */
  intent?: ChatIntent
}

interface UseChatResult {
  messages: ChatMessage[]
  /** True while we're waiting on the agent (drives the typing indicator). */
  typing: boolean
  /** Latest extracted fields (from the most recent agent reply). */
  extracted: ExtractedFields
  send: (text: string) => void
  /** The active conversation id once the first message has been sent (else null). */
  conversationId: string | null
  /**
   * Per-conversation ownership token from the latest chat response (null before
   * the first reply). The seller-photo control echoes it back on upload.
   */
  conversationToken: string | null
  /** Resolved vertical the UI can branch on (defaults to angrosist). */
  vertical: ChatVertical
  /** Resolved intent the UI can branch on (defaults to buy). */
  intent: ChatIntent
}

/**
 * Shared chat engine for the standalone page and the embeddable widget.
 *
 * De-dupe / fallback strategy (so the Vercel demo keeps working without SSE):
 *
 *  1. On send we append the user bubble, turn on the typing indicator, and POST
 *     to /api/chat. The POST always returns the full reply synchronously.
 *  2. We open ONE SSE subscription per conversation (using the id from the first
 *     POST response) and reuse it for every later message.
 *  3. If the SSE connection has opened (`onOpen` fired) we treat SSE as the
 *     source of truth: replies are rendered from `message` events and the POST
 *     reply is ignored. This gives real-time streaming on the long-running
 *     server.
 *  4. If SSE has NOT connected within `SSE_FALLBACK_MS` (or never connects, as
 *     on Vercel), we render the POST reply instead. A per-request guard
 *     (`pendingPostReplyRef`) ensures exactly one of {SSE message, POST reply}
 *     is rendered for each turn — never both.
 */
const SSE_FALLBACK_MS = 1500

export function useChat({
  convStorageKey,
  greeting,
  errorMessage = 'A apărut o eroare. Vă rugăm încercați din nou.',
  vertical = 'angrosist',
  intent = 'buy',
}: UseChatOptions): UseChatResult {
  const [messages, setMessages] = useState<ChatMessage[]>([
    { role: 'assistant', content: greeting },
  ])
  const [typing, setTyping] = useState(false)
  const [extracted, setExtracted] = useState<ExtractedFields>({})
  // Mirrors convIdRef into render state so the upload control can target it.
  const [conversationId, setConversationId] = useState<string | null>(
    () => sessionStorage.getItem(convStorageKey),
  )
  // The per-conversation ownership token, mirrored to state so the seller-photo
  // control can read it. Persisted alongside the id so a reload that resumes the
  // conversation can still authorize continuing turns / SSE before the next reply.
  const tokenStorageKey = `${convStorageKey}_token`
  const [conversationToken, setConversationToken] = useState<string | null>(
    () => sessionStorage.getItem(tokenStorageKey),
  )

  const convIdRef = useRef<string | null>(sessionStorage.getItem(convStorageKey))
  const tokenRef = useRef<string | null>(sessionStorage.getItem(tokenStorageKey))
  const unsubscribeRef = useRef<(() => void) | null>(null)
  const sseOpenRef = useRef(false)

  // Holds the POST reply for the in-flight turn until we know whether SSE will
  // deliver it. Resolving this (via fallback timer or SSE message) renders the
  // reply at most once and clears the guard.
  const pendingPostReplyRef = useRef<StreamMessage | null>(null)
  const fallbackTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const appendAssistant = useCallback((msg: StreamMessage) => {
    setMessages((prev) => [...prev, { role: 'assistant', content: msg.reply }])
    setExtracted(msg.extracted ?? {})
    setTyping(false)
  }, [])

  // Renders the buffered POST reply if SSE didn't get there first.
  const flushPostFallback = useCallback(() => {
    if (fallbackTimerRef.current) {
      clearTimeout(fallbackTimerRef.current)
      fallbackTimerRef.current = null
    }
    const pending = pendingPostReplyRef.current
    if (pending) {
      pendingPostReplyRef.current = null
      appendAssistant(pending)
    }
  }, [appendAssistant])

  const ensureSubscription = useCallback(
    (conversationId: string) => {
      if (unsubscribeRef.current) return
      unsubscribeRef.current = subscribeToConversation(
        conversationId,
        {
          onOpen: () => {
            sseOpenRef.current = true
          },
          onTyping: () => setTyping(true),
          onMessage: (msg) => {
            // SSE won the race: drop the buffered POST reply for this turn so we
            // don't double-render, then render the streamed reply.
            pendingPostReplyRef.current = null
            if (fallbackTimerRef.current) {
              clearTimeout(fallbackTimerRef.current)
              fallbackTimerRef.current = null
            }
            appendAssistant(msg)
          },
          onError: () => {
            // SSE failed for this turn — fall back to the POST reply if buffered.
            if (pendingPostReplyRef.current) {
              flushPostFallback()
            } else {
              setTyping(false)
            }
          },
        },
        {
          // Captured from the chat response just before this call; the backend
          // 403s the stream without it (EventSource can't send headers).
          token: tokenRef.current,
        },
      )
    },
    [appendAssistant, flushPostFallback],
  )

  const send = useCallback(
    (raw: string) => {
      const text = raw.trim()
      if (!text || typing) return

      setMessages((prev) => [...prev, { role: 'user', content: text }])
      setTyping(true)

      void (async () => {
        try {
          // vertical/intent are only honored on the first message (no id yet).
          // The token (null on the first turn) authorizes continuing turns.
          const resp = await sendMessage(
            convIdRef.current,
            text,
            { vertical, intent },
            tokenRef.current,
          )
          convIdRef.current = resp.conversation_id
          setConversationId(resp.conversation_id)
          sessionStorage.setItem(convStorageKey, resp.conversation_id)

          // Capture the ownership token issued on EVERY turn. Read it back into
          // refs/state/storage BEFORE opening SSE so the stream gets `?token=`.
          if (resp.conversation_token) {
            tokenRef.current = resp.conversation_token
            setConversationToken(resp.conversation_token)
            sessionStorage.setItem(tokenStorageKey, resp.conversation_token)
          }

          // Open SSE once we have a conversation id; reuse for later turns.
          ensureSubscription(resp.conversation_id)

          const postReply: StreamMessage = {
            reply: resp.reply,
            state: resp.state,
            extracted: resp.extracted ?? {},
          }

          if (sseOpenRef.current) {
            // SSE is live — let the `message` event render the reply. Still
            // buffer the POST reply as a safety net in case no event arrives.
            pendingPostReplyRef.current = postReply
            fallbackTimerRef.current = setTimeout(flushPostFallback, SSE_FALLBACK_MS)
          } else {
            // SSE not connected yet (e.g. first turn, or Vercel where it never
            // connects). Buffer, then render the POST reply after a short grace
            // period unless an SSE message/open beats the timer.
            pendingPostReplyRef.current = postReply
            fallbackTimerRef.current = setTimeout(flushPostFallback, SSE_FALLBACK_MS)
          }
        } catch {
          pendingPostReplyRef.current = null
          if (fallbackTimerRef.current) {
            clearTimeout(fallbackTimerRef.current)
            fallbackTimerRef.current = null
          }
          setTyping(false)
          setMessages((prev) => [
            ...prev,
            { role: 'assistant', content: errorMessage },
          ])
        }
      })()
    },
    [
      typing,
      convStorageKey,
      tokenStorageKey,
      ensureSubscription,
      flushPostFallback,
      errorMessage,
      vertical,
      intent,
    ],
  )

  // Close the EventSource and clear timers on unmount.
  useEffect(() => {
    return () => {
      unsubscribeRef.current?.()
      unsubscribeRef.current = null
      if (fallbackTimerRef.current) clearTimeout(fallbackTimerRef.current)
    }
  }, [])

  return {
    messages,
    typing,
    extracted,
    send,
    conversationId,
    conversationToken,
    vertical,
    intent,
  }
}
