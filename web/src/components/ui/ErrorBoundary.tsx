import { Component, type ErrorInfo, type ReactNode } from 'react'
import { AlertTriangle, RotateCw } from 'lucide-react'
import { t } from '@/i18n'
import { Button } from './index'

interface Props {
  /** Navigating to a different route clears the error boundary automatically. */
  resetKey?: string
  onHome?: () => void
  children: ReactNode
}

interface State {
  error: string | null
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: unknown): State {
    return { error: error instanceof Error ? error.message : String(error) }
  }

  componentDidUpdate(prevProps: Props): void {
    if (this.state.error !== null && prevProps.resetKey !== this.props.resetKey) {
      this.setState({ error: null })
    }
  }

  componentDidCatch(error: unknown, info: ErrorInfo): void {
    console.error('Studio render error:', error, info.componentStack)
  }

  render() {
    if (this.state.error === null) return this.props.children
    const { onHome } = this.props
    return (
      <div className="flex h-full items-center justify-center p-6">
        <div className="w-full max-w-lg rounded-card border border-line bg-surface p-5">
          <div className="flex items-center gap-2 text-danger">
            <AlertTriangle className="size-4 shrink-0" />
            <h2 className="text-lg font-semibold text-text">{t('common.crashTitle')}</h2>
          </div>
          <p className="mt-1.5 text-sm leading-relaxed text-muted">{t('common.crashGuidance')}</p>
          <pre className="mt-3 max-h-40 overflow-auto whitespace-pre-wrap break-words rounded-ctl border border-line bg-bg p-2.5 font-mono text-xs text-danger">
            {this.state.error}
          </pre>
          <div className="mt-4 flex flex-wrap gap-2">
            <Button variant="primary" icon={RotateCw} onClick={() => this.setState({ error: null })}>
              {t('common.retry')}
            </Button>
            {onHome && (
              <Button
                onClick={() => {
                  this.setState({ error: null })
                  onHome()
                }}
              >
                {t('common.goOverview')}
              </Button>
            )}
          </div>
        </div>
      </div>
    )
  }
}
