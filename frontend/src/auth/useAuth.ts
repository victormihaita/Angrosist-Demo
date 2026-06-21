import { useContext } from 'react'
import { AuthContext, type AuthContextValue } from '@/auth/authContext'

/** Access the dashboard auth state. Throws if used outside <AuthProvider>. */
export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within <AuthProvider>')
  return ctx
}
