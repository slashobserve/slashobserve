import type { components } from "@/api/schema.gen"

type Segment = components["schemas"]["SearchSnippetSegment"]

export function plainText(text: string): string {
  return text
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\*\*|~~|`/g, "")
    .replace(/(^|\n)\s{0,3}(#{1,6}|>|[-*+])\s+/g, "$1")
}

export function plainSegments(segments: readonly Segment[]): Segment[] {
  return segments.map((segment) => ({
    ...segment,
    text: plainText(segment.text),
  }))
}
