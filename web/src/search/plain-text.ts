import type { components } from "@/api/schema.gen"

type Segment = components["schemas"]["SearchSnippetSegment"]

const blockMarker = String.raw`(?:\s{0,3}(?:#{1,6}|>|[-*+]|\d+[.)])\s+)+`

export function plainText(text: string, startsLine = true): string {
  const marker = new RegExp(
    startsLine ? `(^|\n)${blockMarker}` : `(\n)${blockMarker}`,
    "g"
  )
  return text
    .replace(/(!?)\[([^\]]*)\]\((?:[^()]|\([^()]*\))*\)/g, "$2")
    .replace(/\*\*|~~|`/g, "")
    .replace(/\*(?=\S)([^*\n]*?\S)\*/g, "$1")
    .replace(/(^|\W)_(?=\S)([^_\n]*?\S)_(?!\w)/g, "$1$2")
    .replace(marker, "$1")
}

export function plainSegments(segments: readonly Segment[]): Segment[] {
  return segments.map((segment, index) => ({
    ...segment,
    text: plainText(segment.text, index === 0),
  }))
}
