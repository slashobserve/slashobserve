import { Fragment } from "react"

import type { components } from "@/api/schema.gen"

import { mentionName, mentionSegments } from "./mentions"

type Mention = components["schemas"]["Mention"]

export function MessageBody({
  body,
  mentions,
}: {
  body: string
  mentions: readonly Mention[]
}) {
  return (
    <>
      {mentionSegments(body).map((segment, index) => {
        if (segment.kind === "text") {
          return <Fragment key={index}>{segment.text}</Fragment>
        }
        const name = mentionName(segment, mentions)
        if (name === null) return <Fragment key={index}>{segment.raw}</Fragment>
        return (
          <span
            key={index}
            data-mention={segment.ref.kind}
            className="rounded-sm bg-muted px-1 font-medium"
          >
            @{name}
          </span>
        )
      })}
    </>
  )
}
