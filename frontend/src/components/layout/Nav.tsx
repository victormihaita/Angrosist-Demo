import { Link, useLocation } from 'react-router-dom'
import { LogOut } from 'lucide-react'
import { cn } from '@/lib/utils'
import { EmbedDialog } from '@/components/dashboard/EmbedDialog'
import { useAuth } from '@/auth/useAuth'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { t } from '@/lib/strings'

const LINKS = [
  { to: '/', label: 'Acasă', exact: true },
  { to: '/chat', label: 'Chat', exact: true },
  { to: '/dashboard', label: 'Dashboard', exact: false },
]

// Dashboard sub-navigation — only rendered on /dashboard* routes. `exact` keeps
// Pipeline from staying active on the Companies/Handoffs sections.
const DASHBOARD_LINKS = [
  { to: '/dashboard', label: t.nav.pipeline },
  { to: '/dashboard/companies', label: t.nav.companies },
  { to: '/dashboard/handoffs', label: t.nav.handoffs },
]

function initials(name: string): string {
  return name
    .split(' ')
    .map((p) => p[0])
    .filter(Boolean)
    .slice(0, 2)
    .join('')
    .toUpperCase()
}

export function Nav() {
  const { pathname } = useLocation()
  const { user, logout } = useAuth()
  const onDashboard = pathname.startsWith('/dashboard')

  function isActive(to: string, exact: boolean) {
    return exact ? pathname === to : pathname.startsWith(to)
  }

  // Sub-nav active state: Companies / Handoffs match by prefix; Pipeline owns
  // /dashboard and the lead detail (/dashboard/:id), i.e. anything under
  // /dashboard that isn't a companies/handoffs section.
  function isSubActive(to: string) {
    if (to === '/dashboard') {
      return (
        !pathname.startsWith('/dashboard/companies') &&
        !pathname.startsWith('/dashboard/handoffs')
      )
    }
    return pathname.startsWith(to)
  }

  return (
    <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80 shrink-0">
      <div className="max-w-7xl mx-auto px-4 h-14 flex items-center justify-between gap-4">
        {/* Logo */}
        <Link
          to="/"
          className="font-semibold text-sm shrink-0 hover:opacity-80 transition-opacity"
        >
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

        {/* Right actions */}
        <div className="flex items-center gap-2 shrink-0">
          <div className="hidden sm:block">
            <EmbedDialog />
          </div>

          {/* User menu — only meaningful on the authenticated dashboard. */}
          {onDashboard && user && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  className="gap-2"
                  aria-label="Meniu utilizator"
                >
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-primary text-primary-foreground text-[10px] font-semibold">
                    {initials(user.name || user.email)}
                  </span>
                  <span className="hidden md:inline max-w-[140px] truncate">
                    {user.name || user.email}
                  </span>
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-56">
                <DropdownMenuLabel className="flex flex-col gap-0.5">
                  <span className="truncate">{user.name || user.email}</span>
                  <span className="text-xs font-normal text-muted-foreground capitalize">
                    {user.role}
                  </span>
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={logout} variant="destructive">
                  <LogOut className="h-4 w-4" />
                  {t.auth.logout}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      </div>

      {/* Dashboard sub-navigation — only on /dashboard* routes. */}
      {onDashboard && (
        <div className="border-t bg-muted/30">
          <nav className="max-w-7xl mx-auto px-4 h-10 flex items-center gap-0.5">
            {DASHBOARD_LINKS.map(({ to, label }) => (
              <Link
                key={to}
                to={to}
                className={cn(
                  'px-3 py-1 rounded-md text-sm font-medium transition-colors whitespace-nowrap',
                  isSubActive(to)
                    ? 'bg-background text-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground hover:bg-background/60',
                )}
              >
                {label}
              </Link>
            ))}
          </nav>
        </div>
      )}
    </header>
  )
}
