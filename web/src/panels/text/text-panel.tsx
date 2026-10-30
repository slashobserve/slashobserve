import { Fragment, useMemo } from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"

import { textDefaults } from "@/spec/defaults"
import type { TextPanel as TextPanelSpec } from "@/spec"

import type { StaticPanelRenderer } from "../types"
import { components, markdownRehypePlugins, renderHtml } from "./html"
import { interpolateText } from "./interpolate"
import { safeUrl } from "./safe-url"

function CodeView({
  text,
  code,
}: {
  text: string
  code: TextPanelSpec["code"]
}) {
  const lines = text.split("\n")
  return (
    <pre
      data-language={code?.language}
      className="my-2 overflow-auto rounded-control border p-2 first:mt-0 last:mb-0"
    >
      {code?.lineNumbers ? (
        <code className="grid grid-cols-[auto_1fr] font-mono text-xs">
          {lines.map((line, index) => (
            <Fragment key={index}>
              <span
                aria-hidden="true"
                className="pr-3 text-right text-muted-foreground select-none"
              >
                {index + 1}
              </span>
              <span className="whitespace-pre">{line}</span>
            </Fragment>
          ))}
        </code>
      ) : (
        <code className="font-mono text-xs">{text}</code>
      )}
    </pre>
  )
}

const TextPanel: StaticPanelRenderer = ({ panel, vars }) => {
  const content = panel.type === "text" ? panel.content : ""
  const mode = (panel.type === "text" && panel.mode) || textDefaults.mode
  const code = panel.type === "text" ? panel.code : undefined
  const text = useMemo(
    () => interpolateText(content, vars, mode),
    [content, vars, mode]
  )
  const html = useMemo(
    () => (mode === "html" ? renderHtml(text) : null),
    [mode, text]
  )
  return (
    <div
      data-text-panel
      data-mode={mode}
      className="isolate size-full overflow-auto p-3 pt-0 [contain:paint]"
    >
      {mode === "markdown" ? (
        <Markdown
          remarkPlugins={[remarkGfm]}
          rehypePlugins={markdownRehypePlugins}
          urlTransform={(url) => safeUrl(url)}
          components={components}
        >
          {text}
        </Markdown>
      ) : mode === "html" ? (
        html
      ) : (
        <CodeView text={text} code={code} />
      )}
    </div>
  )
}

export default TextPanel
