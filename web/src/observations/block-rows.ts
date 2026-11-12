import { ApiError } from "@/api/errors"
import type { DataPanel } from "@/spec"

import { blockWriteProblem } from "./errors"
import type { Block, Observation } from "./queries"

export type BlockRow = { key: string; blocks: [Block] | [Block, Block] }

export function blockRows(blocks: readonly Block[]): BlockRow[] {
  const sorted = blocks.toSorted((a, b) => a.position - b.position)
  const rows: BlockRow[] = []
  for (let i = 0; i < sorted.length; i++) {
    const block = sorted[i]
    const next = sorted[i + 1]
    if (block.width === "half" && next !== undefined && next.width === "half") {
      rows.push({ key: block.id, blocks: [block, next] })
      i++
    } else {
      rows.push({ key: block.id, blocks: [block] })
    }
  }
  return rows
}

export function blockTitle(block: Block): string {
  switch (block.kind) {
    case "text":
      return "Text"
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

export function nextBlockId(
  blocks: readonly Block[],
  removedId: string
): string | null {
  const sorted = blocks.toSorted((a, b) => a.position - b.position)
  const index = sorted.findIndex((block) => block.id === removedId)
  if (index === -1) return null
  return sorted[index + 1]?.id ?? null
}

export function newPanelBody(
  panel: DataPanel,
  range: Pick<Observation, "from" | "to">
): { panel: Record<string, unknown>; from?: string; to?: string } {
  const copy: Record<string, unknown> = { ...panel }
  delete copy.id
  delete copy.gridPos
  if (range.from !== null && range.to !== null) {
    return { panel: copy, from: range.from, to: range.to }
  }
  return { panel: copy }
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
