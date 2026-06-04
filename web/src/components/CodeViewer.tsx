// Adapted from deepseek-reasonix (MIT) — components/CodeViewer.tsx
// (single "editor seam": heavy editor lazy-loaded behind Suspense with a <pre>
// fallback). We target Monaco (@monaco-editor/react) instead of highlight.js.
import { lazy, Suspense } from 'react'
import { ExternalLink } from 'lucide-react'
import { t } from '../lib/i18n'
import styles from './CodeViewer.module.css'

export interface CodeViewerProps {
  path?: string
  content?: string
  binary?: boolean
  truncated?: boolean
  onReveal?: (path: string) => void
}

// LazyMonaco streams @monaco-editor/react in on first text-file view. It is a
// SEPARATE module so the test suite (jsdom) never imports monaco: the <pre>
// fallback below is what Suspense shows until the chunk resolves, and under
// jsdom that chunk is never awaited. The real Vite build resolves it (Monaco was
// added to web/package.json by Wave 2; if absent, add @monaco-editor/react +
// monaco-editor to devDependencies — see Notes).
const LazyMonaco = lazy(() => import('./MonacoCode'))

export function CodeViewer({
  path = '',
  content = '',
  binary = false,
  truncated = false,
  onReveal,
}: CodeViewerProps) {
  return (
    <div className={styles.viewer}>
      {path && (
        <div className={styles.bar}>
          <span className={styles.path}>{path}</span>
          <button
            className={styles.reveal}
            onClick={() => onReveal?.(path)}
            title={t('workspace.reveal', 'Reveal in OS')}
          >
            <ExternalLink size={13} aria-hidden />
            {t('workspace.reveal', 'Reveal')}
          </button>
        </div>
      )}

      {!path ? (
        <p className={styles.notice}>{t('workspace.pickFile', 'Select a file to view its contents.')}</p>
      ) : binary ? (
        <p className={styles.notice}>{t('workspace.binary', 'Binary file — preview unavailable.')}</p>
      ) : (
        <>
          {truncated && (
            <p className={`${styles.notice} ${styles.warn}`}>
              {t('workspace.truncated', 'File truncated for preview.')}
            </p>
          )}
          <Suspense
            fallback={
              <pre className={styles.fallback} data-testid="code-fallback">
                {content}
              </pre>
            }
          >
            <LazyMonaco path={path} content={content} />
          </Suspense>
        </>
      )}
    </div>
  )
}
