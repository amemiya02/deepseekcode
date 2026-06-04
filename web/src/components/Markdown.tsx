// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/Markdown.tsx
// react-markdown + remark-gfm (tables/task-lists/strike) + remark-math + rehype-katex
// ($inline$/$$block$$) + rehype-highlight (highlight.js fenced-code highlighting).
// Fenced code blocks render as a styled <pre><code>; inline code is a styled <code>.
import ReactMarkdown from 'react-markdown'
import type { Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import rehypeKatex from 'rehype-katex'
import rehypeHighlight from 'rehype-highlight'
import 'katex/dist/katex.min.css'
import 'highlight.js/styles/github-dark.css'

const components: Components = {
  a: ({ href, children }) => (
    <a href={href} target="_blank" rel="noreferrer noopener">{children}</a>
  ),
  code: ({ className, children, ...rest }) => (
    <code className={`md-code ${className ?? ''}`} {...rest}>{children}</code>
  ),
}

// LLMs emit \( \) \[ \] delimiters (remark-math only parses $/$$); convert them,
// but protect LaTeX line-break spacing \\[ from the rewrite.
export function normalizeMath(s: string): string {
  const lb = '\x00LB\x00'
  let r = s.replace(/\\\\\[/g, lb)
  r = r
    .replace(/\\\[/g, () => '$$')
    .replace(/\\\]/g, () => '$$')
    .replace(/\\\(/g, () => '$')
    .replace(/\\\)/g, () => '$')
  r = r.replace(/\x00LB\x00/g, '\\\\[')
  return r
}

export function Markdown({ text }: { text: string }) {
  return (
    <div className="md markdown-body">
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkMath]}
        rehypePlugins={[rehypeKatex, rehypeHighlight]}
        components={components}
      >
        {normalizeMath(text)}
      </ReactMarkdown>
    </div>
  )
}
