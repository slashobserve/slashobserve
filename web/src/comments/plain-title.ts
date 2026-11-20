import { mentionSegments } from "./mentions"

export function plainTitle(title: string): string {
  return mentionSegments(title)
    .map((segment) =>
      segment.kind === "text" ? segment.text : `@${segment.name}`
    )
    .join("")
}
