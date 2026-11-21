import { maxText } from "@/conversation/anchor"
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
  return clip(selected.replace(/\s+/g, " ").trim()).slice(0, maxText).trim()
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

export function findQuote(root: Node, quote: string): Range | null {
  const needle = quote.replace(/\s+/g, "")
  if (needle === "") return null
  const index = indexText(root)
  const start = index.text.indexOf(needle)
  if (start < 0) return null
  const first = index.at[start]
  const last = index.at[start + needle.length - 1]
  const range = document.createRange()
  range.setStart(first.node, first.offset)
  range.setEnd(last.node, last.offset + 1)
  return range
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

function locate(layout: TextLayout, anchor: Anchor): Rect | null {
  if (anchor.kind !== "text" || anchor.quote === undefined) return null
  const range = findQuote(layout.root, anchor.quote)
  if (range === null) return null
  const surface = layout.root.closest("[data-canvas-surface]") ?? layout.root
  return rangeRect(range, surface) ?? rectWithin(layout.root, surface)
}

export const textAnchors: AnchorsModule = {
  hitTest: () => null,
  locate: locate as NonNullable<AnchorsModule["locate"]>,
}
