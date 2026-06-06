// Adapted from deepseek-reasonix (MIT) — components/Markdown.tsx.
// Markdown via react-markdown + remark-gfm (tables, task lists, strike, autolinks)
// and remark-math + rehype-katex for $inline$/$$block$$ KaTeX. Fenced code is
// highlighted with rehype-highlight (highlight.js) so no heavy editor lands in the
// initial bundle. Wave 2 can swap fenced rendering to the Monaco seam later; this
// <Markdown> API is the stable contract those waves import.
import ReactMarkdown from 'react-markdown'
import type { Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import rehypeKatex from 'rehype-katex'
import rehypeHighlight from 'rehype-highlight'
import 'katex/dist/katex.min.css'
import 'highlight.js/styles/github-dark.css'

const components: Components = {
  // Open links in the system browser when a native bridge exists; otherwise a
  // normal anchor. Wave 6's bridge can override this; Wave 0 keeps it inert-safe.
  a: ({ href, children }) => (
    <a href={href} target="_blank" rel="noreferrer noopener">
      {children}
    </a>
  ),
}

// LLMs emit \( \) \[ \] delimiters (remark-math only parses $/$$); convert them,
// but protect LaTeX line-break spacing \\[ from the rewrite, and swap | for \vert
// inside math so remark-gfm can't read the bar as a table column.
function normalizeMath(s: string): string {
  const lb = '\x00LB\x00'
  let r = s.replace(/\\\\\[/g, lb)
  r = r
    .replace(/\\\[/g, () => '$$')
    .replace(/\\\]/g, () => '$$')
    .replace(/\\\(/g, () => '$')
    .replace(/\\\)/g, () => '$')
  r = r.replace(/\x00LB\x00/g, '\\\\[')
  const vert = (m: string) => m.replace(/\|/g, '\\vert ')
  r = r.replace(/\$\$([\s\S]*?)\$\$/g, (_m, m) => `$$${vert(m)}$$`)
  r = r.replace(/\$([^$\n]+)\$/g, (_m, m) => `$${vert(m)}$`)
  return r
}

export function Markdown({ text }: { text: string }) {
  return (
    <div className="md">
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
