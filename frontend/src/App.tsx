import { lazy, Suspense } from 'react'
import { BrowserRouter, Routes, Route, useLocation } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Nav } from '@/components/layout/Nav'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { AuthProvider } from '@/auth/AuthProvider'
import { ProtectedRoute } from '@/auth/ProtectedRoute'
import { Toaster } from '@/components/ui/sonner'
import { LandingPage } from '@/pages/LandingPage'
import { ChatPage } from '@/pages/ChatPage'

// Code-split the authenticated dashboard so the public chat/landing bundle stays
// small and the dashboard only loads for operators who reach it.
const DashboardPage = lazy(() =>
  import('@/pages/DashboardPage').then((m) => ({ default: m.DashboardPage })),
)
const LeadDetailPage = lazy(() =>
  import('@/pages/LeadDetailPage').then((m) => ({ default: m.LeadDetailPage })),
)
const LoginPage = lazy(() =>
  import('@/pages/LoginPage').then((m) => ({ default: m.LoginPage })),
)

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1, // one retry on transient failures, then surface the error
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    },
  },
})

function RouteFallback() {
  return <div className="flex flex-1 items-center justify-center py-20" />
}

function AppLayout() {
  const { pathname } = useLocation()
  const isChatPage = pathname === '/chat'

  return (
    // h-dvh shrinks when the virtual keyboard appears on mobile,
    // keeping the input pinned above it.
    <div className="flex flex-col h-dvh">
      <Nav />
      <main
        className={
          isChatPage
            ? 'flex flex-col flex-1 overflow-hidden'
            : 'flex flex-col flex-1 overflow-y-auto'
        }
      >
        <Suspense fallback={<RouteFallback />}>
          <Routes>
            <Route path="/" element={<LandingPage />} />
            <Route path="/chat" element={<ChatPage />} />
            <Route path="/login" element={<LoginPage />} />
            <Route
              path="/dashboard"
              element={
                <ProtectedRoute>
                  <DashboardPage />
                </ProtectedRoute>
              }
            />
            <Route
              path="/dashboard/:id"
              element={
                <ProtectedRoute>
                  <LeadDetailPage />
                </ProtectedRoute>
              }
            />
          </Routes>
        </Suspense>
      </main>
    </div>
  )
}

export default function App() {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <AuthProvider>
            <AppLayout />
            <Toaster richColors position="top-right" />
          </AuthProvider>
        </BrowserRouter>
      </QueryClientProvider>
    </ErrorBoundary>
  )
}
