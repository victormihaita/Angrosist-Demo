import { Component, type ErrorInfo, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'

interface Props {
  children: ReactNode
}

interface State {
  hasError: boolean
}

/**
 * App-level error boundary. Catches render-time errors anywhere in the tree and
 * shows a friendly fallback instead of a blank white screen. Network/data errors
 * from TanStack Query are handled at the query/page level; this is the last
 * line of defense for unexpected render failures.
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }

  static getDerivedStateFromError(): State {
    return { hasError: true }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // TODO(M5): forward to Sentry once observability is wired.
    console.error('Unhandled UI error:', error, info)
  }

  handleReload = () => {
    this.setState({ hasError: false })
    window.location.reload()
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex flex-1 flex-col items-center justify-center gap-4 p-8 text-center">
          <h1 className="text-lg font-semibold">Ceva nu a funcționat</h1>
          <p className="text-sm text-muted-foreground">
            A apărut o eroare neașteptată. Reîncărcați pagina pentru a continua.
          </p>
          <Button onClick={this.handleReload}>Reîncarcă</Button>
        </div>
      )
    }
    return this.props.children
  }
}
