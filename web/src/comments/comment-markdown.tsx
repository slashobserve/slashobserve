import { memo, useMemo } from "react"
import Markdown, { type Components } from "react-markdown"
import remarkGfm from "remark-gfm"
import type { Plugin } from "unified"

import {
  commentRehypePlugins,
  components,
  type HastElement,
  type HastNode,
  type HastRoot,
} from "@/panels/text/html"
import { safeUrl } from "@/panels/text/safe-url"

export type Chip = { label: string; kind: string | null }

const open = ""
const close = ""
const markerPattern = /(\d+)/g
const reserved = /[]/g

export function markerFor(index: number): string {
  return `${open}${index}${close}`
}

export function withoutMarkers(text: string): string {
  return text.replace(reserved, "")
}

const remarkPlugins = [remarkGfm]

const commentComponents: Components = {
  ...components,
  ...(components.h3 === undefined
    ? {}
    : { h1: components.h3, h2: components.h3 }),
  img: ({ alt }) => <>{alt}</>,
  p: ({ children }) => (
    <p className="my-1 whitespace-pre-line first:mt-0 last:mb-0">{children}</p>
  ),
  li: ({ children }) => <li className="whitespace-pre-line">{children}</li>,
}

type TextNode = Extract<HastNode, { type: "text" }>
type Child = HastElement["children"][number]

function chipElement(chip: Chip): HastElement {
  const properties: HastElement["properties"] = {
    className: [
      "rounded-chip",
      "bg-primary-container",
      "px-[3px]",
      "font-semibold",
      "text-on-primary-container",
    ],
  }
  if (chip.kind !== null) properties["data-mention"] = chip.kind
  return {
    type: "element",
    tagName: "span",
    properties,
    children: [{ type: "text", value: `@${chip.label}` }],
  }
}

function split(node: TextNode, chips: readonly Chip[], inCode: boolean) {
  const out: Child[] = []
  let from = 0
  for (const match of node.value.matchAll(markerPattern)) {
    const chip = chips[Number(match[1])]
    if (match.index > from) {
      out.push({ type: "text", value: node.value.slice(from, match.index) })
    }
    if (chip !== undefined) {
      out.push(
        inCode ? { type: "text", value: `@${chip.label}` } : chipElement(chip)
      )
    }
    from = match.index + match[0].length
  }
  if (from < node.value.length) {
    out.push({ type: "text", value: node.value.slice(from) })
  }
  return out
}

function chipsPlugin(chips: readonly Chip[]): Plugin<[], HastRoot> {
  return () => (tree) => {
    const visit = (node: HastRoot | HastElement, inCode: boolean) => {
      const code =
        inCode || (node.type === "element" && node.tagName === "code")
      const children: Child[] = []
      for (const child of node.children) {
        if (child.type === "text") {
          children.push(...split(child, chips, code))
          continue
        }
        if (child.type === "element") visit(child, code)
        children.push(child as Child)
      }
      node.children = children
    }
    visit(tree, false)
  }
}

export const CommentMarkdown = memo(function CommentMarkdown({
  markdown,
  chips,
}: {
  markdown: string
  chips: readonly Chip[]
}) {
  const rehypePlugins = useMemo(
    () => [...commentRehypePlugins, chipsPlugin(chips)],
    [chips]
  )
  return (
    <Markdown
      remarkPlugins={remarkPlugins}
      rehypePlugins={rehypePlugins}
      urlTransform={(url) => safeUrl(url)}
      components={commentComponents}
    >
      {markdown}
    </Markdown>
  )
})
