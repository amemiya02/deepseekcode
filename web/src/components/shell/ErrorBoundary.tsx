// Adapted from deepseek-reasonix (MIT) — components/ErrorBoundary.tsx.
import { Component, type ReactNode } from 'react'
import { t } from '../../lib/i18n'
import { IconAlertTriangle, IconRefresh } from '../../lib/icons'
import styles from './index.module.css'

interface State {
  error: Error | null
}

export class ErrorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: unknown) {
    // Hook point for crash reporting (Wave 6). Keep a console record meanwhile.
    console.error('ErrorBoundary caught', error)
  }

  reset = () => this.setState({ error: null })

  render() {
    const { error } = this.state
    if (!error) return this.props.children
    return (
      <div className={styles.errorFallback} role="alert">
        <IconAlertTriangle size={20} />
        <h2>{t('error.title', 'Something went wrong')}</h2>
        <p className={styles.errorDetail}>{error.message}</p>
        <button className={styles.errorRetry} onClick={this.reset}>
          <IconRefresh size={14} />
          {t('error.retry', 'Retry')}
        </button>
      </div>
    )
  }
}
