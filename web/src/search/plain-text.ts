import type { components } from "@/api/schema.gen"

type Segment = components["schemas"]["SearchSnippetSegment"]

export function plainText(text: string, startsLine = true): string {
  const blockMarker = startsLine
    ? /(^|\n)\s{0,3}(#{1,6}|>|[-*+])\s+/g
    : /(\n)\s{0,3}(#{1,6}|>|[-*+])\s+/g
  return text
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\*\*|~~|`/g, "")
    .replace(blockMarker, "$1")
}

export function plainSegments(segments: readonly Segment[]): Segment[] {
  return segments.map((segment, index) => ({
    ...segment,
    text: plainText(segment.text, index === 0),
  }))
}
