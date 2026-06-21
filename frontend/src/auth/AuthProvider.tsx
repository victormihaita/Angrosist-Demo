import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AuthContext } from '@/auth/authContext'
import {
  getStoredToken,
  getStoredUser,
  setStoredAuth,
  clearStoredAuth,
  type AuthUser,
} from '@/lib/authStore'
import { login as apiLogin, setUnauthorizedHandler } from '@/lib/api'

/**
 * Provides dashboard auth state (user + token) backed by localStorage, and wires
 * the API client's 401 handler so any expired/invalid token bounces the operator
 * to /login. The public chat + widget never mount this provider, so they stay
 * fully unauthenticated.
 */
export function AuthProvider({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate()
  const [token, setToken] = useState<string | null>(() => getStoredToken())
  const [user, setUser] = useState<AuthUser | null>(() => getStoredUser())

  // Keep the latest navigate in a ref so the registered 401 handler is stable.
  const navigateRef = useRef(navigate)
  useEffect(() => {
    navigateRef.current = navigate
  }, [navigate])

  useEffect(() => {
    setUnauthorizedHandler(() => {
      setToken(null)
      setUser(null)
      navigateRef.current('/login', { replace: true })
    })
    return () => setUnauthorizedHandler(null)
  }, [])

  const login = useCallback(async (email: string, password: string) => {
    const res = await apiLogin(email, password)
    setStoredAuth(res.token, res.user)
    setToken(res.token)
    setUser(res.user)
  }, [])

  const logout = useCallback(() => {
    clearStoredAuth()
    setToken(null)
    setUser(null)
    navigateRef.current('/login', { replace: true })
  }, [])

  const value = useMemo(
    () => ({ user, token, login, logout }),
    [user, token, login, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
