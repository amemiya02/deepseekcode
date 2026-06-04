// Adapted from deepseek-reasonix (MIT) — components/CodeViewer.tsx (editor seam).
// Single seam for all code/diff views. Monaco is lazy + memoized so the ~MB editor
// streams in only when first needed; loadMonaco() lets callers warm it ahead of use.
// PlainCode is the always-available fallback (and the Linux/WebKitGTK escape hatch).
import { Suspense, lazy, type ReactElement } from 'react'

export interface MonacoMountProps {
  value: string
  language?: string
  readOnly?: boolean
  /** Render a unified diff: original vs value. When set, a diff editor is used. */
  original?: string
  height?: number | string
}

// PlainCode: dependency-free fallback used while Monaco loads, on platforms where
// the webview can't run it, or in tests. Waves 2/5 reuse this as their LCS fallback.
export function PlainCode({ value, language }: { value: string; language?: string }): ReactElement {
  return (
    <pre className="plain-code" data-language={language}>
      <code>{value}</code>
    </pre>
  )
}

// Lazy React component that pulls in @monaco-editor/react on first render only.
const LazyMonaco = lazy(async () => {
  const mod = await import('@monaco-editor/react')
  const Editor = mod.default
  const Diff = mod.DiffEditor
  const Inner = (props: MonacoMountProps) => {
    if (props.original !== undefined) {
      return (
        <Diff
          original={props.original}
          modified={props.value}
          language={props.language}
          height={props.height ?? 360}
          options={{ readOnly: props.readOnly ?? true, renderSideBySide: false }}
        />
      )
    }
    return (
      <Editor
        value={props.value}
        language={props.language}
        height={props.height ?? 360}
        options={{ readOnly: props.readOnly ?? true, minimap: { enabled: false } }}
      />
    )
  }
  return { default: Inner }
})

// MonacoMount wraps the lazy editor with a PlainCode Suspense fallback.
export function MonacoMount(props: MonacoMountProps): ReactElement {
  return (
    <Suspense fallback={<PlainCode value={props.value} language={props.language} />}>
      <LazyMonaco {...props} />
    </Suspense>
  )
}

// loadMonaco warms the editor module + Monaco engine ahead of first render so the
// first diff view is instant. Safe to call multiple times (import is memoized).
let warmed: Promise<unknown> | null = null
export function loadMonaco(): Promise<unknown> {
  if (!warmed) {
    warmed = import('@monaco-editor/react').then((mod) => mod.loader.init())
  }
  return warmed
}
