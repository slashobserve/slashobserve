import {
  clip,
  positionOf,
  type Anchor,
  type AnchorsModule,
  type Point,
  type Rect,
  type Size,
} from "@/panels/anchors"
import { rectWithin } from "@/panels/anchor-layout"

export type TextLayout = { root: HTMLElement; markdown: string }

export function quoteOf(selected: string): string {
  return clip(selected.replace(/\s+/g, " ").trim()).trim()
}

export function textAnchor(quote: string, at: Point, body: Size): Anchor {
  return { kind: "text", pos: positionOf(at, body), quote }
}

type TextIndex = { text: string; at: { node: Text; offset: number }[] }

const space = /\s/

function indexText(root: Node): TextIndex {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  const index: TextIndex = { text: "", at: [] }
  for (let node = walker.nextNode(); node !== null; node = walker.nextNode()) {
    const value = node.nodeValue ?? ""
    for (let offset = 0; offset < value.length; offset += 1) {
      if (space.test(value[offset])) continue
      index.text += value[offset]
      index.at.push({ node: node as Text, offset })
    }
  }
  return index
}

function rangeOf(index: TextIndex, start: number, length: number): Range {
  const first = index.at[start]
  const last = index.at[start + length - 1]
  const range = document.createRange()
  range.setStart(first.node, first.offset)
  range.setEnd(last.node, last.offset + 1)
  return range
}

export function findQuotes(root: Node, quote: string): Range[] {
  const needle = quote.replace(/\s+/g, "")
  if (needle === "") return []
  const index = indexText(root)
  const found: Range[] = []
  for (
    let start = index.text.indexOf(needle);
    start >= 0;
    start = index.text.indexOf(needle, start + 1)
  ) {
    found.push(rangeOf(index, start, needle.length))
  }
  return found
}

export function rangeRect(range: Range, root: Element): Rect | null {
  const first = range.getClientRects()[0] ?? range.getBoundingClientRect()
  const origin = root.getBoundingClientRect()
  if (first.width === 0 && first.height === 0) return null
  return {
    x: first.left - origin.left,
    y: first.top - origin.top,
    width: first.width,
    height: first.height,
  }
}

function distance(rect: Rect, target: Point): number {
  return Math.hypot(rect.x - target.x, rect.y + rect.height - target.y)
}

function nearest(rects: readonly Rect[], target: Point): Rect | null {
  let best: Rect | null = null
  for (const rect of rects) {
    if (best === null || distance(rect, target) < distance(best, target)) {
      best = rect
    }
  }
  return best
}

function locate(layout: TextLayout, anchor: Anchor): Rect | null {
  if (anchor.kind !== "text" || anchor.quote === undefined) return null
  const ranges = findQuotes(layout.root, anchor.quote)
  if (ranges.length === 0) return null
  const surface =
    layout.root.closest<HTMLElement>("[data-canvas-surface]") ?? layout.root
  const target = {
    x: anchor.pos.x * surface.offsetWidth,
    y: anchor.pos.y * surface.offsetHeight,
  }
  const rects = ranges.flatMap((range) => rangeRect(range, surface) ?? [])
  return nearest(rects, target) ?? rectWithin(layout.root, surface)
}

export const textAnchors: AnchorsModule = {
  hitTest: () => null,
  locate: locate as NonNullable<AnchorsModule["locate"]>,
}
