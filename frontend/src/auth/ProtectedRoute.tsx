import { Navigate, useLocation } from 'react-router-dom'
import { useAuth } from '@/auth/useAuth'

/**
 * Gates the /dashboard routes. Without a token we redirect to /login, preserving
 * the attempted path so a successful login can return there. The backend RBAC is
 * the real gate; this is purely UX.
 */
export function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { token } = useAuth()
  const location = useLocation()

  if (!token) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />
  }
  return <>{children}</>
}
