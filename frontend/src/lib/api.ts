const API_URL = import.meta.env.VITE_API_URL ?? ''

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
  const res = await fetch(`${API_URL}/api/chat`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ conversation_id: conversationId ?? undefined, message }),
  })
  if (!res.ok) throw new Error(`chat error ${res.status}`)
  return res.json()
}

export async function getLeads(): Promise<Lead[]> {
  const res = await fetch(`${API_URL}/api/leads`)
  if (!res.ok) throw new Error(`leads error ${res.status}`)
  return res.json()
}

export async function getLead(id: string): Promise<LeadDetail> {
  const res = await fetch(`${API_URL}/api/leads/${id}`)
  if (!res.ok) throw new Error(`lead error ${res.status}`)
  return res.json()
}
