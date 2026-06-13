import { Link, useLocation } from 'react-router-dom'
import { cn } from '@/lib/utils'
import { EmbedDialog } from '@/components/dashboard/EmbedDialog'

const LINKS = [
  { to: '/',          label: 'Acasă',     exact: true },
  { to: '/chat',      label: 'Chat',      exact: true },
  { to: '/dashboard', label: 'Dashboard', exact: false },
]

export function Nav() {
  const { pathname } = useLocation()

  function isActive(to: string, exact: boolean) {
    return exact ? pathname === to : pathname.startsWith(to)
  }

  return (
    <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80 shrink-0">
      <div className="max-w-6xl mx-auto px-4 h-14 flex items-center justify-between gap-4">
        {/* Logo */}
        <Link to="/" className="font-semibold text-sm shrink-0 hover:opacity-80 transition-opacity">
          Euro Intermed
        </Link>

        {/* Nav links */}
        <nav className="flex items-center gap-0.5">
          {LINKS.map(({ to, label, exact }) => (
            <Link
              key={to}
              to={to}
              className={cn(
                'px-3 py-1.5 rounded-md text-sm font-medium transition-colors whitespace-nowrap',
                isActive(to, exact)
                  ? 'bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:text-foreground hover:bg-muted',
              )}
            >
              {label}
            </Link>
          ))}
        </nav>

        {/* Right actions — hidden on very small screens */}
        <div className="hidden sm:block shrink-0">
          <EmbedDialog />
        </div>
      </div>
    </header>
  )
}
