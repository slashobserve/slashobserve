import { ApiError } from "@/api/errors"
import { plainText } from "@/search/plain-text"
import type { DataPanel, Variable } from "@/spec"

import { blockWriteProblem } from "./errors"
import type { Block, Observation } from "./queries"

export type BlockWidth = Block["width"]

export type BlockRow = { key: string; blocks: Block[] }

export const widthSteps: readonly BlockWidth[] = [
  "third",
  "half",
  "two_thirds",
  "full",
]

const columnsOf: Record<BlockWidth, number> = {
  third: 2,
  half: 3,
  two_thirds: 4,
  full: 6,
}

const shareOf: Record<BlockWidth, number> = {
  third: 1 / 3,
  half: 1 / 2,
  two_thirds: 2 / 3,
  full: 1,
}

const rowColumns = 6

export const singleColumnBelow = 640

export const spanClass: Record<BlockWidth, string> = {
  third: "@min-[640px]:col-span-2",
  half: "@min-[640px]:col-span-3",
  two_thirds: "@min-[640px]:col-span-4",
  full: "@min-[640px]:col-span-6",
}

export const rowGridClass = "grid grid-cols-1 gap-4 @min-[640px]:grid-cols-6"

const minBlockHeight = 120
const maxBlockHeight = 960
const heightStep = 20
const keyHeightStep = 40

export function packRows<T extends { width: BlockWidth }>(
  items: readonly T[]
): T[][] {
  const rows: T[][] = []
  let used = 0
  for (const item of items) {
    const columns = columnsOf[item.width]
    const last = rows.at(-1)
    if (last !== undefined && used + columns <= rowColumns) {
      last.push(item)
      used += columns
    } else {
      rows.push([item])
      used = columns
    }
  }
  return rows
}

export function blockRows(blocks: readonly Block[]): BlockRow[] {
  const sorted = blocks.toSorted((a, b) => a.position - b.position)
  return packRows(sorted).map((row) => ({ key: row[0].id, blocks: row }))
}

function defaultBlockHeight(width: BlockWidth): number {
  return width === "full" || width === "two_thirds" ? 280 : 220
}

export function blockHeight(block: {
  width: BlockWidth
  height?: number | null
}): number {
  return block.height ?? defaultBlockHeight(block.width)
}

export function hasHeight(block: Block): boolean {
  return block.kind === "panel" || block.kind === "snapshot"
}

function clampHeight(pixels: number): number {
  return Math.min(maxBlockHeight, Math.max(minBlockHeight, pixels))
}

export function snapHeight(pixels: number): number {
  return clampHeight(Math.round(pixels / heightStep) * heightStep)
}

export function snapWidth(pixels: number, rowPixels: number): BlockWidth {
  if (rowPixels <= 0) return "full"
  const share = pixels / rowPixels
  let nearest: BlockWidth = "full"
  for (const width of widthSteps) {
    if (Math.abs(shareOf[width] - share) < Math.abs(shareOf[nearest] - share)) {
      nearest = width
    }
  }
  return nearest
}

export function stepWidth(width: BlockWidth, direction: -1 | 1): BlockWidth {
  const index = widthSteps.indexOf(width) + direction
  return widthSteps[Math.min(widthSteps.length - 1, Math.max(0, index))]
}

export function stepHeight(pixels: number, direction: -1 | 1): number {
  return clampHeight(pixels + direction * keyHeightStep)
}

export type BlockChange = { width?: BlockWidth; height?: number | null }

export type Draft = { order: string[]; changes: Record<string, BlockChange> }

export function orderOf(blocks: readonly Block[]): string[] {
  return blocks.toSorted((a, b) => a.position - b.position).map((b) => b.id)
}

export function moveBlock(
  order: readonly string[],
  id: string,
  to: number
): string[] {
  const from = order.indexOf(id)
  if (from === -1) return [...order]
  const rest = order.filter((entry) => entry !== id)
  rest.splice(Math.min(rest.length, Math.max(0, to)), 0, id)
  return rest
}

export function stepBlock(
  order: readonly string[],
  id: string,
  direction: -1 | 1
): string[] {
  const from = order.indexOf(id)
  if (from === -1) return [...order]
  return moveBlock(order, id, from + direction)
}

export type DropTarget = { id: string; after: boolean; sideBySide: boolean }

export type BlockRect = {
  id: string
  left: number
  top: number
  width: number
  height: number
}

export function dropTarget(
  rects: readonly BlockRect[],
  point: { x: number; y: number },
  draggedId: string,
  rowPixels: number
): DropTarget | null {
  const hit = rects.find(
    (rect) =>
      point.x >= rect.left &&
      point.x <= rect.left + rect.width &&
      point.y >= rect.top &&
      point.y <= rect.top + rect.height
  )
  if (hit === undefined || hit.id === draggedId) return null
  const sideBySide = hit.width < rowPixels * 0.95
  const after = sideBySide
    ? point.x > hit.left + hit.width / 2
    : point.y > hit.top + hit.height / 2
  return { id: hit.id, after, sideBySide }
}

export function placeBeside(
  order: readonly string[],
  id: string,
  target: Pick<DropTarget, "id" | "after">
): string[] {
  if (id === target.id) return [...order]
  const rest = order.filter((entry) => entry !== id)
  const at = rest.indexOf(target.id)
  if (at === -1) return [...order]
  rest.splice(target.after ? at + 1 : at, 0, id)
  return rest
}

export function applyDraft(blocks: readonly Block[], draft: Draft): Block[] {
  const byId = new Map(blocks.map((block) => [block.id, block]))
  const known = draft.order.filter((id) => byId.has(id))
  const named = new Set(known)
  const rest = orderOf(blocks).filter((id) => !named.has(id))
  return [...known, ...rest].map((id, position) => {
    const block = byId.get(id) as Block
    const change = draft.changes[id]
    return {
      ...block,
      position,
      width: change?.width ?? block.width,
      height: change?.height === undefined ? block.height : change.height,
    }
  })
}

export function arrangeBody(draft: Draft): {
  blocks: { id: string; width?: BlockWidth; height?: number | null }[]
} {
  return {
    blocks: draft.order.map((id) => ({ id, ...draft.changes[id] })),
  }
}

const textTitleWords = 6
const textTitleLength = 48
const ellipsis = "\u2026"
const thematicBreak = /^\s{0,3}([-*_])(\s*\1){2,}\s*$/
const markersOnly = /^(?:(?:#{1,6}|>|[-*+]|\d+[.)])\s*)+$/

function textBlockTitle(markdown: unknown): string {
  if (typeof markdown !== "string") return "Text"
  const line = markdown
    .split("\n")
    .filter((entry) => !thematicBreak.test(entry))
    .map((entry) => plainText(entry).trim())
    .find((entry) => entry !== "" && !markersOnly.test(entry))
  if (line === undefined) return "Text"
  const words = line.split(/\s+/)
  const clipped = Array.from(words.slice(0, textTitleWords).join(" "))
  if (clipped.length > textTitleLength) {
    return clipped.slice(0, textTitleLength).join("").trimEnd() + ellipsis
  }
  return words.length > textTitleWords
    ? clipped.join("") + ellipsis
    : clipped.join("")
}

export function blockTitle(block: Block): string {
  switch (block.kind) {
    case "text":
      return textBlockTitle(block.body.markdown)
    case "snapshot":
      return "Snapshot"
    case "run":
      return "Run"
    case "panel": {
      const panel = block.body.panel
      if (typeof panel === "object" && panel !== null && "title" in panel) {
        const title = panel.title
        if (typeof title === "string" && title.trim() !== "") return title
      }
      return "Panel"
    }
  }
}

export function citeLabel(
  blocks: readonly Block[],
  blockId: string,
  index: number
): string {
  const cited = blocks.find((block) => block.id === blockId)
  if (cited?.kind === "text") {
    const markdown = cited.body.markdown
    const heading =
      typeof markdown === "string" ? /^#{1,6}\s+(.+)$/m.exec(markdown) : null
    return heading?.[1]?.trim() || "Conclusion"
  }
  if (cited?.kind === "panel") {
    const title = blockTitle(cited)
    if (title !== "Panel") return title
  }
  return index === 0 ? "Conclusion" : `Panel ${index}`
}

export function holdsAgentOutput(block: Block): boolean {
  return (
    block.kind === "run" ||
    block.created_by.run_id !== null ||
    block.created_by.origin === "mcp"
  )
}

export function nextBlockId(
  blocks: readonly Block[],
  removedId: string
): string | null {
  const sorted = blocks.toSorted((a, b) => a.position - b.position)
  const index = sorted.findIndex((block) => block.id === removedId)
  if (index === -1) return null
  return sorted[index + 1]?.id ?? null
}

export function followsLivePanel(
  blocks: readonly Block[],
  blockId: string
): boolean {
  const sorted = blocks.toSorted((a, b) => a.position - b.position)
  const index = sorted.findIndex((block) => block.id === blockId)
  const above = index > 0 ? sorted[index - 1] : undefined
  return above?.kind === "panel" && above.state === "live"
}

export function newPanelBody(
  panel: DataPanel,
  range: Pick<Observation, "from" | "to">,
  variables: readonly Variable[] = []
): {
  panel: Record<string, unknown>
  variables?: Variable[]
  from?: string
  to?: string
} {
  const copy: Record<string, unknown> = { ...panel }
  delete copy.id
  delete copy.gridPos
  const body: ReturnType<typeof newPanelBody> = { panel: copy }
  if (variables.length > 0) body.variables = [...variables]
  if (range.from !== null && range.to !== null) {
    body.from = range.from
    body.to = range.to
  }
  return body
}

export function addProblemText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 429) {
      if (error.retryAfterMs === undefined) {
        return "Too many requests right now. Try again shortly."
      }
      const seconds = Math.max(1, Math.ceil(error.retryAfterMs / 1000))
      const unit = seconds === 1 ? "second" : "seconds"
      return `Too many requests right now. Try again in ${seconds} ${unit}.`
    }
    if (error.status === 502) {
      return "A datasource is unavailable, so its data could not be read. Try again."
    }
    if (error.status === 504) {
      return "A datasource did not answer in time, so its data could not be read. Try again."
    }
  }
  return blockWriteProblem(error).text
}
