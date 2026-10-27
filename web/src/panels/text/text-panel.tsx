import { useMemo } from "react"
import { ExternalLinkIcon } from "lucide-react"
import Markdown, { type Components } from "react-markdown"
import remarkGfm from "remark-gfm"

import { cn } from "@/lib/utils"

import type { StaticPanelRenderer } from "../types"
import { interpolateText } from "./interpolate"
import { isExternal, isImageSource, safeUrl } from "./safe-url"

const heading = "mt-2 font-semibold"
const block = "my-2 first:mt-0 last:mb-0"

const components: Components = {
  a({ href, title, children }) {
    if (!href) return <span>{children}</span>
    const external = isExternal(href)
    return (
      <a
        href={href}
        title={title}
        rel="noopener noreferrer"
        target={external ? "_blank" : undefined}
        className="underline underline-offset-2"
      >
        {children}
        {external ? (
          <ExternalLinkIcon
            aria-hidden="true"
            className="ml-0.5 inline size-3 align-baseline"
          />
        ) : null}
      </a>
    )
  },
  img({ src, alt }) {
    if (typeof src !== "string" || !isImageSource(src)) return <>{alt}</>
    return (
      <img
        src={src}
        alt={alt ?? ""}
        loading="lazy"
        referrerPolicy="no-referrer"
        className="max-w-full"
      />
    )
  },
  h1: ({ children }) => (
    <h1 className={cn(heading, "text-[18px]")}>{children}</h1>
  ),
  h2: ({ children }) => (
    <h2 className={cn(heading, "text-[16px]")}>{children}</h2>
  ),
  h3: ({ children }) => (
    <h3 className={cn(heading, "text-[14px]")}>{children}</h3>
  ),
  h4: ({ children }) => (
    <h4 className={cn(heading, "text-[14px]")}>{children}</h4>
  ),
  h5: ({ children }) => (
    <h5 className={cn(heading, "text-[14px]")}>{children}</h5>
  ),
  h6: ({ children }) => (
    <h6 className={cn(heading, "text-[14px]")}>{children}</h6>
  ),
  p: ({ children }) => (
    <p className={cn(block, "text-[14px] leading-normal")}>{children}</p>
  ),
  ul: ({ children }) => (
    <ul className={cn(block, "list-disc pl-[18px]")}>{children}</ul>
  ),
  ol: ({ children }) => (
    <ol className={cn(block, "list-decimal pl-[18px]")}>{children}</ol>
  ),
  code: ({ children }) => <code className="font-mono text-xs">{children}</code>,
  pre: ({ children }) => (
    <pre className={cn(block, "overflow-auto rounded-control border p-2")}>
      {children}
    </pre>
  ),
  table: ({ children }) => (
    <div className={cn(block, "overflow-x-auto")}>
      <table className="border-collapse text-[13px]">{children}</table>
    </div>
  ),
  th: ({ children }) => (
    <th className="border px-2 py-1 text-left font-semibold">{children}</th>
  ),
  td: ({ children }) => <td className="border px-2 py-1">{children}</td>,
  blockquote: ({ children }) => (
    <blockquote className={cn(block, "pl-3 text-muted-foreground")}>
      {children}
    </blockquote>
  ),
}

const TextPanel: StaticPanelRenderer = ({ panel, vars }) => {
  const content = panel.type === "text" ? panel.content : ""
  const text = useMemo(() => interpolateText(content, vars), [content, vars])
  return (
    <div data-text-panel className="size-full overflow-auto p-3 pt-0">
      <Markdown
        remarkPlugins={[remarkGfm]}
        skipHtml
        urlTransform={(url) => safeUrl(url)}
        components={components}
      >
        {text}
      </Markdown>
    </div>
  )
}

export default TextPanel
