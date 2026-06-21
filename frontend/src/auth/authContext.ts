import { createContext } from 'react'
import type { AuthUser } from '@/lib/authStore'

export interface AuthContextValue {
  user: AuthUser | null
  token: string | null
  login: (email: string, password: string) => Promise<void>
  logout: () => void
}

/**
 * Auth context for the dashboard. Lives in its own module (no component export)
 * so the provider file stays Fast-Refresh-friendly.
 */
export const AuthContext = createContext<AuthContextValue | null>(null)
