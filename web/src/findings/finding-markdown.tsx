import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"

import { components, markdownRehypePlugins } from "@/panels/text/html"
import { safeUrl } from "@/panels/text/safe-url"

export function FindingMarkdown({ markdown }: { markdown: string }) {
  return (
    <div className="text-sm leading-[1.55]">
      <Markdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={markdownRehypePlugins}
        urlTransform={(url) => safeUrl(url)}
        components={components}
      >
        {markdown}
      </Markdown>
    </div>
  )
}
