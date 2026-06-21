/**
 * Minimal token persistence for the dashboard session. The token lives in
 * localStorage under a single key so the authed API client (src/lib/api.ts) can
 * read it without importing React, and so a hard reload keeps the operator
 * logged in. No secrets are baked in here — only the per-session JWT the backend
 * issues at login is stored, and it is cleared on logout or any 401.
 */

const TOKEN_KEY = 'angrosist_token'
const USER_KEY = 'angrosist_user'

/** PublicUser mirrors the API `User` schema (openapi `User`). */
export interface AuthUser {
  id: string
  email: string
  name: string
  role: 'staff' | 'admin'
}

export function getStoredToken(): string | null {
  if (typeof window === 'undefined') return null
  return window.localStorage.getItem(TOKEN_KEY)
}

export function getStoredUser(): AuthUser | null {
  if (typeof window === 'undefined') return null
  const raw = window.localStorage.getItem(USER_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw) as AuthUser
  } catch {
    return null
  }
}

export function setStoredAuth(token: string, user: AuthUser): void {
  window.localStorage.setItem(TOKEN_KEY, token)
  window.localStorage.setItem(USER_KEY, JSON.stringify(user))
}

export function clearStoredAuth(): void {
  if (typeof window === 'undefined') return
  window.localStorage.removeItem(TOKEN_KEY)
  window.localStorage.removeItem(USER_KEY)
}
