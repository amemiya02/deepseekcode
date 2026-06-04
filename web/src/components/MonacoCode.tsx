// MonacoCode is the heavy editor behind CodeViewer's seam. It is a default
// export imported via React.lazy, so it lands in its own chunk and never loads
// under jsdom (the test asserts on CodeViewer's <pre> fallback). Read-only;
// content is whole-file (truncation is handled upstream by /v1/file).
import Editor from '@monaco-editor/react'

export interface MonacoCodeProps {
  path: string
  content: string
}

export default function MonacoCode({ path, content }: MonacoCodeProps) {
  return (
    <div style={{ flex: 1, minHeight: 0 }}>
      <Editor
        height="100%"
        path={path}
        value={content}
        options={{ readOnly: true, automaticLayout: true, minimap: { enabled: false } }}
      />
    </div>
  )
}
