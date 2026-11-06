import Markdown, { type Components } from "react-markdown"
import remarkGfm from "remark-gfm"

import { components, markdownRehypePlugins } from "@/panels/text/html"
import { safeUrl } from "@/panels/text/safe-url"

const findingComponents: Components = {
  ...components,
  p: ({ children, style }) => (
    <p style={style} className="my-2 first:mt-0 last:mb-0">
      {children}
    </p>
  ),
}

export function FindingMarkdown({ markdown }: { markdown: string }) {
  return (
    <Markdown
      remarkPlugins={[remarkGfm]}
      rehypePlugins={markdownRehypePlugins}
      urlTransform={(url) => safeUrl(url)}
      components={findingComponents}
    >
      {markdown}
    </Markdown>
  )
}
