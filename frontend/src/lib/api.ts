/**
 * Resolves the backend base URL consistently across the app and the embeddable
 * widget. The widget injects `window.__ANGROSIST_API_URL__` at runtime so a
 * single `widget.js` can target different backends per host site; that override
 * wins. Otherwise we fall back to the build-time `VITE_API_URL` (empty string =
 * same-origin). No host is ever hardcoded.
 */
export function getApiBase(): string {
  if (typeof window !== 'undefined') {
    const override = (window as unknown as Record<string, unknown>)
      .__ANGROSIST_API_URL__
    if (typeof override === 'string' && override) return override
  }
  return import.meta.env.VITE_API_URL ?? ''
}

export interface ExtractedFields {
  product_name?: string
  quantity?: number
  unit?: string
  delivery_location?: string
  cui?: string
  company_name?: string
  phone?: string
  email?: string
}

export interface ChatResponse {
  conversation_id: string
  reply: string
  state: string
  extracted: ExtractedFields
}

export interface Lead {
  id: string
  status: string
  company_name: string
  cui: string
  product_name: string
  quantity: number | null
  unit: string
  delivery_location: string
  created_at: string
}

export interface TranscriptMessage {
  id: string
  role: string
  content: string
  tool_calls?: string   // base64-encoded JSON array of Gemini parts
  created_at: string
}

export interface LeadDetail extends Lead {
  address: string
  county: string
  phone: string
  email: string
  transcript: TranscriptMessage[]
}

export async function sendMessage(
  conversationId: string | null,
  message: string,
): Promise<ChatResponse> {
  const res = await fetch(`${getApiBase()}/api/chat`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ conversation_id: conversationId ?? undefined, message }),
  })
  if (!res.ok) throw new Error(`chat error ${res.status}`)
  return res.json()
}

export async function getLeads(): Promise<Lead[]> {
  const res = await fetch(`${getApiBase()}/api/leads`)
  if (!res.ok) throw new Error(`leads error ${res.status}`)
  return res.json()
}

export async function getLead(id: string): Promise<LeadDetail> {
  const res = await fetch(`${getApiBase()}/api/leads/${id}`)
  if (!res.ok) throw new Error(`lead error ${res.status}`)
  return res.json()
}

// ---------------------------------------------------------------------------
// SSE: real-time agent replies + typing indicator (M2 Epic 2.3)
// ---------------------------------------------------------------------------

/** Payload delivered to `onMessage` — normalized from the Go-JSON `message` event. */
export interface StreamMessage {
  reply: string
  state: string
  extracted: ExtractedFields
}

export interface StreamHandlers {
  /** Agent started working — show the typing indicator. */
  onTyping?: () => void
  /** Agent produced a reply — append it, update extracted fields, hide typing. */
  onMessage?: (msg: StreamMessage) => void
  /** Stream-level or agent error — show a friendly message, hide typing. */
  onError?: (message: string) => void
  /** EventSource connection opened — used to decide SSE-vs-POST de-dupe. */
  onOpen?: () => void
}

export interface StreamOptions {
  /** Custom EventSource factory (testing). Defaults to the global EventSource. */
  eventSourceFactory?: (url: string) => EventSource
}

/**
 * Shape of the backend `message` event. Go's encoding/json keeps exported field
 * names capitalized, so the wire payload uses `Reply`/`State`/`Extracted`.
 */
interface GoMessageEvent {
  Reply?: string
  State?: string
  Extracted?: ExtractedFields | null
  Error?: string
  Type?: string
}

/**
 * Opens an SSE subscription for a conversation and dispatches named events to
 * the provided handlers. Returns an unsubscribe function that closes the
 * EventSource. Heartbeat comments (`: ping`) are ignored by EventSource itself.
 */
export function subscribeToConversation(
  conversationId: string,
  handlers: StreamHandlers,
  options?: StreamOptions,
): () => void {
  const url = `${getApiBase()}/api/stream?conversation_id=${encodeURIComponent(
    conversationId,
  )}`
  const es = options?.eventSourceFactory
    ? options.eventSourceFactory(url)
    : new EventSource(url)

  es.addEventListener('open', () => handlers.onOpen?.())

  es.addEventListener('typing', () => handlers.onTyping?.())

  es.addEventListener('message', (e) => {
    try {
      const data = JSON.parse((e as MessageEvent).data) as GoMessageEvent
      if (data.Error) {
        handlers.onError?.(data.Error)
        return
      }
      handlers.onMessage?.({
        reply: data.Reply ?? '',
        state: data.State ?? '',
        extracted: data.Extracted ?? {},
      })
    } catch {
      handlers.onError?.('stream parse error')
    }
  })

  es.addEventListener('error', (e) => {
    // The native EventSource `error` event carries no JSON; the backend's
    // application-level `error` event does. Try to parse, else generic message.
    const raw = (e as MessageEvent).data
    if (typeof raw === 'string' && raw) {
      try {
        const data = JSON.parse(raw) as GoMessageEvent
        handlers.onError?.(data.Error ?? 'agent error')
        return
      } catch {
        /* fall through */
      }
    }
    handlers.onError?.('connection error')
  })

  return () => es.close()
}
