import { BrowserRouter, Routes, Route, useLocation } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Nav } from '@/components/layout/Nav'
import { LandingPage } from '@/pages/LandingPage'
import { ChatPage } from '@/pages/ChatPage'
import { DashboardPage } from '@/pages/DashboardPage'
import { LeadDetailPage } from '@/pages/LeadDetailPage'

const queryClient = new QueryClient()

function AppLayout() {
  const { pathname } = useLocation()
  const isChatPage = pathname === '/chat'

  return (
    // h-dvh shrinks when the virtual keyboard appears on mobile,
    // keeping the input pinned above it.
    <div className="flex flex-col h-dvh">
      <Nav />
      <main className={
        isChatPage
          ? 'flex flex-col flex-1 overflow-hidden'
          : 'flex flex-col flex-1 overflow-y-auto'
      }>
        <Routes>
          <Route path="/"              element={<LandingPage />} />
          <Route path="/chat"          element={<ChatPage />} />
          <Route path="/dashboard"     element={<DashboardPage />} />
          <Route path="/dashboard/:id" element={<LeadDetailPage />} />
        </Routes>
      </main>
    </div>
  )
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AppLayout />
      </BrowserRouter>
    </QueryClientProvider>
  )
}
