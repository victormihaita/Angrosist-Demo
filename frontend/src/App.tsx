import { BrowserRouter, Routes, Route, useLocation } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Nav } from '@/components/layout/Nav'
import { ChatPage } from '@/pages/ChatPage'
import { DashboardPage } from '@/pages/DashboardPage'
import { LeadDetailPage } from '@/pages/LeadDetailPage'

const queryClient = new QueryClient()

function AppLayout() {
  const { pathname } = useLocation()
  const isChatPage = pathname === '/'

  return (
    <div className="flex flex-col min-h-screen">
      <Nav />
      <main className={isChatPage ? 'flex flex-col flex-1 overflow-hidden' : 'flex flex-col flex-1'}>
        <Routes>
          <Route path="/" element={<ChatPage />} />
          <Route path="/dashboard" element={<DashboardPage />} />
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
