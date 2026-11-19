import { Fragment, useEffect, useMemo, useRef, useState } from "react"
import { ChevronDownIcon } from "lucide-react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"

import { textDefaults } from "@/spec/defaults"
import type { TextPanel as TextPanelSpec } from "@/spec"

import type { StaticPanelRenderer } from "../types"
import { components, markdownRehypePlugins, renderHtml } from "./html"
import { interpolateText } from "./interpolate"
import { safeUrl } from "./safe-url"
import { hasMoreBelow } from "./scroll-hint"

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
  const box = useRef<HTMLDivElement>(null)
  const [more, setMore] = useState(false)
  useEffect(() => {
    const element = box.current
    if (!element) return
    const update = () => setMore(hasMoreBelow(element))
    update()
    element.addEventListener("scroll", update, { passive: true })
    const observer = new ResizeObserver(update)
    observer.observe(element)
    if (element.firstElementChild) observer.observe(element.firstElementChild)
    return () => {
      element.removeEventListener("scroll", update)
      observer.disconnect()
    }
  }, [text, mode])
  return (
    <div
      ref={box}
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
      {more ? (
        <div className="sticky bottom-0 h-0 overflow-visible">
          <button
            type="button"
            onClick={() =>
              box.current?.scrollBy({
                top: box.current.clientHeight,
                behavior: "smooth",
              })
            }
            className="absolute right-0 bottom-0 flex items-center gap-1 rounded-control border border-border bg-background px-2 py-0.5 text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            More
            <ChevronDownIcon className="size-3" aria-hidden="true" />
          </button>
        </div>
      ) : null}
    </div>
  )
}

export default TextPanel
