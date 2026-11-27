import { Fragment, memo, useMemo } from "react"

import type { components } from "@/api/schema.gen"

import {
  CommentMarkdown,
  markerFor,
  withoutMarkers,
  type Chip,
} from "./comment-markdown"
import { mentionName, mentionSegments } from "./mentions"

type Mention = components["schemas"]["Mention"]

function literal(raw: string): string {
  return withoutMarkers(raw).replace(/[\\[\]()]/g, "\\$&")
}

function markdownOf(body: string, mentions: readonly Mention[]) {
  const chips: Chip[] = []
  const markdown = mentionSegments(body)
    .map((segment) => {
      if (segment.kind === "text") return withoutMarkers(segment.text)
      const name = mentionName(segment, mentions)
      if (name === null) return literal(segment.raw)
      chips.push({ label: name, kind: segment.ref.kind })
      return markerFor(chips.length - 1)
    })
    .join("")
  return { markdown, chips }
}

const codeSpan = /`([^`\n]+)`/g

function WithCodeSpans({ text }: { text: string }) {
  const parts = text.split(codeSpan)
  return (
    <>
      {parts.map((part, index) =>
        index % 2 === 0 ? (
          <Fragment key={index}>{part}</Fragment>
        ) : (
          <code key={index} className="font-mono text-xs">
            {part}
          </code>
        )
      )}
    </>
  )
}

export const MessageBody = memo(function MessageBody({
  body,
  mentions,
  markdown = false,
  codeSpans = false,
}: {
  body: string
  mentions: readonly Mention[]
  markdown?: boolean
  codeSpans?: boolean
}) {
  const parsed = useMemo(
    () => (markdown ? markdownOf(body, mentions) : null),
    [markdown, body, mentions]
  )
  if (parsed !== null) {
    return <CommentMarkdown markdown={parsed.markdown} chips={parsed.chips} />
  }
  return (
    <>
      {mentionSegments(body).map((segment, index) => {
        if (segment.kind === "text") {
          return codeSpans ? (
            <WithCodeSpans key={index} text={segment.text} />
          ) : (
            <Fragment key={index}>{segment.text}</Fragment>
          )
        }
        const name = mentionName(segment, mentions)
        if (name === null) return <Fragment key={index}>{segment.raw}</Fragment>
        return (
          <span
            key={index}
            data-mention={segment.ref.kind}
            className="rounded-chip bg-primary-container px-[3px] font-semibold text-on-primary-container"
          >
            @{name}
          </span>
        )
      })}
    </>
  )
})
