import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"

import { components, markdownRehypePlugins } from "../../text/html"
import { safeUrl } from "../../text/safe-url"

export function MarkdownCell({
  value,
  grows,
}: {
  value: unknown
  grows: boolean
}) {
  if (value === null || value === undefined || value === "") {
    return <span className="text-muted-foreground">-</span>
  }
  return (
    <div
      data-markdown-cell=""
      className={
        grows ? undefined : "max-h-4 overflow-hidden whitespace-nowrap"
      }
    >
      <Markdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={markdownRehypePlugins}
        urlTransform={(url) => safeUrl(url)}
        components={components}
      >
        {String(value)}
      </Markdown>
    </div>
  )
}
