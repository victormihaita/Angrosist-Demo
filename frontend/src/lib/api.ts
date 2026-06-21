import {
  getStoredToken,
  clearStoredAuth,
  type AuthUser,
} from '@/lib/authStore'

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

export interface TranscriptMessage {
  id: string
  role: string
  content: string
  tool_calls?: string   // base64-encoded JSON array of Gemini parts
  created_at: string
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

// ===========================================================================
// Authenticated dashboard API (M3 Epic 3.3)
//
// These helpers attach the staff JWT and centralize error handling against the
// documented envelope `{error:{code,message,details}}`. On 401 anywhere we clear
// the token and let a registered handler bounce the operator to /login. The
// public chat/SSE helpers above are intentionally left untouched (unauthed).
// ===========================================================================

/** Structured error surfaced to callers; carries the stable backend code. */
export class ApiError extends Error {
  status: number
  code: string
  details?: { field: string; issue: string }[]
  constructor(
    status: number,
    code: string,
    message: string,
    details?: { field: string; issue: string }[],
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.details = details
  }
}

/**
 * Registered by the auth provider so the fetch wrapper can trigger a redirect on
 * 401 without importing React. Keeping it as a module-level callback avoids
 * coupling the data layer to the router.
 */
let onUnauthorized: (() => void) | null = null
export function setUnauthorizedHandler(fn: (() => void) | null): void {
  onUnauthorized = fn
}

interface ErrorEnvelope {
  error?: {
    code?: string
    message?: string
    details?: { field: string; issue: string }[]
  }
}

/**
 * fetch wrapper for authed dashboard calls. Injects the Bearer token, parses the
 * error envelope, and on 401 clears the token + fires the unauthorized handler
 * (redirect to /login). Throws ApiError on any non-2xx.
 */
async function authedFetch<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const token = getStoredToken()
  const headers = new Headers(init?.headers)
  headers.set('Content-Type', 'application/json')
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(`${getApiBase()}/api${path}`, { ...init, headers })

  if (res.status === 401) {
    clearStoredAuth()
    onUnauthorized?.()
    throw new ApiError(401, 'UNAUTHENTICATED', 'Sesiune expirată. Autentifică-te din nou.')
  }

  if (!res.ok) {
    let code = 'INTERNAL'
    let message = `Eroare ${res.status}`
    let details: { field: string; issue: string }[] | undefined
    try {
      const body = (await res.json()) as ErrorEnvelope
      if (body.error) {
        code = body.error.code ?? code
        message = body.error.message ?? message
        details = body.error.details
      }
    } catch {
      /* non-JSON error body — keep defaults */
    }
    throw new ApiError(res.status, code, message, details)
  }

  // 204 / empty body tolerant
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

// --- Auth -----------------------------------------------------------------

export interface LoginResponse {
  token: string
  expires_at?: string
  user: AuthUser
}

export async function login(
  email: string,
  password: string,
): Promise<LoginResponse> {
  // Login itself is unauthed but shares the envelope/error handling.
  return authedFetch<LoginResponse>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
}

// --- Leads (paginated, filtered) ------------------------------------------

export type LeadStatus =
  | 'new'
  | 'qualifying'
  | 'needs_human'
  | 'qualified'
  | 'offer_requested'
  | 'offer_sent'
  | 'negotiation'
  | 'won'
  | 'lost'

export type Vertical = 'angrosist' | 'palletclearance' | 'skalyou'

/** LeadSummary mirrors domain.LeadSummary / openapi LeadSummary. */
export interface LeadSummary {
  id: string
  status: string
  company_name: string
  cui: string
  product_name: string
  quantity: number | null
  unit: string
  delivery_location: string
  created_at: string
  vertical: string
  assigned_to: string | null
  needs_human: boolean
  offer_value: number | null
  offer_note?: string
}

export interface PageInfo {
  next_cursor: string | null
  limit: number
  count: number
}

export interface LeadListPage {
  data: LeadSummary[]
  page: PageInfo
}

export interface LeadFilters {
  status?: string
  vertical?: string
  assigned_to?: string
  q?: string
  cursor?: string
  limit?: number
}

export async function listLeads(filters: LeadFilters): Promise<LeadListPage> {
  const params = new URLSearchParams()
  if (filters.status) params.set('status', filters.status)
  if (filters.vertical) params.set('vertical', filters.vertical)
  if (filters.assigned_to) params.set('assigned_to', filters.assigned_to)
  if (filters.q) params.set('q', filters.q)
  if (filters.cursor) params.set('cursor', filters.cursor)
  if (filters.limit) params.set('limit', String(filters.limit))
  const qs = params.toString()
  return authedFetch<LeadListPage>(`/leads${qs ? `?${qs}` : ''}`)
}

// --- Lead detail ----------------------------------------------------------

export interface SourcingRequestView {
  lead_id?: string
  product: string
  quantity?: number | null
  unit?: string
  delivery_location?: string
  recurring?: boolean
  budget?: number | null
}

export interface CompanyVerificationView {
  source?: string
  vat_status?: string | null
  administrators?: string[]
  checked_at?: string
}

export interface LeadCompanyView {
  id: string
  name: string
  cui: string
  country: string
  reg_no?: string
  caen?: string
  vat_status?: string
  roles?: string[]
  verification?: CompanyVerificationView | null
}

export interface LeadContactView {
  id?: string
  name: string
  phone: string
  email: string
}

export interface AuthedLeadDetail extends LeadSummary {
  address?: string
  county?: string
  phone?: string
  email?: string
  intent?: string
  summary?: string
  transcript: TranscriptMessage[]
  sourcing_request?: SourcingRequestView | null
  company?: LeadCompanyView | null
  contact?: LeadContactView | null
}

export async function getLeadDetail(id: string): Promise<AuthedLeadDetail> {
  return authedFetch<AuthedLeadDetail>(`/leads/${encodeURIComponent(id)}`)
}

// --- Offer tracking + assignment ------------------------------------------

export interface OfferUpdate {
  status?: string
  value?: number
  note?: string
}

export async function updateOffer(
  id: string,
  body: OfferUpdate,
): Promise<LeadSummary> {
  return authedFetch<LeadSummary>(`/leads/${encodeURIComponent(id)}/offer`, {
    method: 'PATCH',
    body: JSON.stringify(body),
  })
}

export async function assignLead(
  id: string,
  userId: string | null,
): Promise<LeadSummary> {
  return authedFetch<LeadSummary>(`/leads/${encodeURIComponent(id)}/assign`, {
    method: 'POST',
    body: JSON.stringify({ user_id: userId }),
  })
}

// --- Users (admin; degrades gracefully on 403) ----------------------------

export interface PublicUser {
  id: string
  email: string
  name: string
  role: 'staff' | 'admin'
}

export async function listUsers(): Promise<PublicUser[]> {
  return authedFetch<PublicUser[]>('/users')
}

// --- B2B directory (companies) --------------------------------------------

export type CompanyRole =
  | 'distributor'
  | 'importer'
  | 'wholesaler'
  | 'retailer'
  | 'horeca'
  | 'processor'
  | 'producer'
  | 'buyer'
  | 'seller'

/** CompanySummary mirrors domain.CompanySummary / openapi Company. */
export interface CompanySummary {
  id: string
  name: string
  cui: string
  country: string
  reg_no: string
  caen: string
  vat_status: string
  roles: string[]
  created_at: string
}

export interface CompanyListPage {
  data: CompanySummary[]
  page: PageInfo
}

export interface CompanyFilters {
  role?: string
  country?: string
  q?: string
  cursor?: string
  limit?: number
}

export async function listCompanies(
  filters: CompanyFilters,
): Promise<CompanyListPage> {
  const params = new URLSearchParams()
  if (filters.role) params.set('role', filters.role)
  if (filters.country) params.set('country', filters.country)
  if (filters.q) params.set('q', filters.q)
  if (filters.cursor) params.set('cursor', filters.cursor)
  if (filters.limit) params.set('limit', String(filters.limit))
  const qs = params.toString()
  return authedFetch<CompanyListPage>(`/companies${qs ? `?${qs}` : ''}`)
}

export interface CompanyFinancialView {
  year: number
  turnover: number | null
}

/** CompanyDetail mirrors domain.CompanyDetail / openapi CompanyDetail. */
export interface CompanyDetail extends CompanySummary {
  address?: string
  county?: string
  is_active?: boolean
  verification?: CompanyVerificationView | null
  financials?: CompanyFinancialView[]
}

export async function getCompany(id: string): Promise<CompanyDetail> {
  return authedFetch<CompanyDetail>(`/companies/${encodeURIComponent(id)}`)
}

// --- Handoff queue ---------------------------------------------------------

/** HandoffItem mirrors domain.HandoffItem / openapi HandoffItem. */
export interface HandoffItem {
  id: string
  status: string
  vertical: string
  company_name: string
  product_name: string
  assigned_to: string | null
  last_message: string
  created_at: string
}

export interface HandoffListPage {
  data: HandoffItem[]
  page: PageInfo
}

export async function listHandoffs(): Promise<HandoffListPage> {
  return authedFetch<HandoffListPage>('/handoffs')
}

// --- KPIs ------------------------------------------------------------------

/** Kpis mirrors domain.KPIs / openapi Kpis. */
export interface Kpis {
  offers_sent: number
  won: number
  qualified: number
  conversion_rate: number
  pipeline_value: number
}

export async function getKpis(): Promise<Kpis> {
  return authedFetch<Kpis>('/kpis')
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
