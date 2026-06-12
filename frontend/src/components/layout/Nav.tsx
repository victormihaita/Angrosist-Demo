import { Link, useLocation } from 'react-router-dom'
import { cn } from '@/lib/utils'
import { EmbedDialog } from '@/components/dashboard/EmbedDialog'

export function Nav() {
  const { pathname } = useLocation()

  return (
    <header className="border-b bg-background shrink-0">
      <div className="max-w-7xl mx-auto px-4 h-14 flex items-center justify-between">
        <div className="flex items-center gap-6">
          <span className="font-semibold text-sm">Euro Intermed</span>
          <nav className="flex gap-1">
            <Link
              to="/"
              className={cn(
                'px-3 py-1.5 rounded-md text-sm transition-colors',
                pathname === '/'
                  ? 'bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:text-foreground hover:bg-muted',
              )}
            >
              Chat
            </Link>
            <Link
              to="/dashboard"
              className={cn(
                'px-3 py-1.5 rounded-md text-sm transition-colors',
                pathname.startsWith('/dashboard')
                  ? 'bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:text-foreground hover:bg-muted',
              )}
            >
              Dashboard
            </Link>
          </nav>
        </div>
        <EmbedDialog />
      </div>
    </header>
  )
}
