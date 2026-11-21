import { createContext } from "react"
import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { observationKeys } from "@/api/keys"
import { formatDate, zoneLabel } from "@/lib/date-format"
import { describeRange, liveNowRefresh, refreshToMs } from "@/lib/time"
import type { Block } from "@/observations/queries"
import type { DataPanel, Panel, Variable } from "@/spec"

import { blockRange } from "../panel-input"

export const blockPanelId = "block"

export type PanelBlockBody = {
  panel: Panel
  variables?: Variable[]
  from?: string
  to?: string
}
export type RunBlockBody = { runId: string; cites: string[] }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function optionalString(value: unknown): string | undefined | null {
  if (value === undefined) return undefined
  return typeof value === "string" ? value : null
}

export function panelBlockBody(block: Block): PanelBlockBody | null {
  const { panel, from, to, variables } = block.body
  if (!isRecord(panel) || typeof panel.type !== "string") return null
  const parsedFrom = optionalString(from)
  const parsedTo = optionalString(to)
  if (parsedFrom === null || parsedTo === null) return null
  const body: PanelBlockBody = {
    panel: { ...panel, id: blockPanelId } as unknown as Panel,
  }
  if (Array.isArray(variables) && variables.every(isRecord)) {
    body.variables = variables as unknown as Variable[]
  }
  if (parsedFrom !== undefined) body.from = parsedFrom
  if (parsedTo !== undefined) body.to = parsedTo
  return body
}

export type PageRange = { from: string | null; to: string | null }

export type BlockWindow = {
  from: string
  to: string
  pinned: boolean
  caption: string
  refreshMs: number | null
}

export const liveHour = { from: "now-1h", to: "now" }

export const liveHourLabel = "Last hour, live"

export const PageRangeContext = createContext<PageRange>({
  from: null,
  to: null,
})

export function resolveBlockRange(
  page: PageRange,
  block: Pick<PanelBlockBody, "from" | "to">
): BlockWindow {
  if (block.from !== undefined && block.to !== undefined) {
    return {
      from: block.from,
      to: block.to,
      pinned: true,
      caption: `Pinned to ${describeRange(block.from, block.to)}`,
      refreshMs: null,
    }
  }
  if (page.from !== null && page.to !== null) {
    return {
      from: page.from,
      to: page.to,
      pinned: false,
      caption: describeRange(page.from, page.to),
      refreshMs: null,
    }
  }
  return {
    ...liveHour,
    pinned: false,
    caption: liveHourLabel,
    refreshMs: refreshToMs(liveNowRefresh),
  }
}

export function pageRangeFor(
  next: { from: string; to: string },
  now: number
): PageRange {
  if (next.from === liveHour.from && next.to === liveHour.to) {
    return { from: null, to: null }
  }
  return blockRange(next.from, next.to, now)
}

export function pinnedBody(
  body: Block["body"],
  from: string,
  to: string,
  now: number
): Block["body"] | null {
  const range = blockRange(from, to, now)
  if (range.from === null || range.to === null) return null
  return { ...body, from: range.from, to: range.to }
}

export function snapshotIdOf(block: Block): string | null {
  const id = block.body.snapshot_id
  return typeof id === "string" && id !== "" ? id : null
}

export function runBlockBody(block: Block): RunBlockBody | null {
  const { run_id: runId, cites } = block.body
  if (typeof runId !== "string" || runId === "") return null
  if (!Array.isArray(cites)) return null
  if (!cites.every((cite) => typeof cite === "string")) return null
  return { runId, cites: cites as string[] }
}

export function datasourceUidOf(panel: DataPanel): string | undefined {
  if (panel.datasource !== undefined) return panel.datasource
  for (const query of panel.queries) {
    if ("datasource" in query && query.datasource !== undefined) {
      return query.datasource
    }
  }
  return undefined
}

const frozenFormat = "YYYY-MM-DD HH:mm"

export function frozenCaption(from: string, to: string): string {
  const start = formatDate(Date.parse(from), frozenFormat)
  const end = formatDate(Date.parse(to), frozenFormat)
  return `Frozen, ${start} to ${end} ${zoneLabel(Date.parse(to))}`
}

export function snapshotQuery(snapshotId: string) {
  return queryOptions({
    queryKey: observationKeys.snapshot(snapshotId),
    queryFn: ({ signal }) =>
      api
        .GET("/snapshots/{id}", {
          params: { path: { id: snapshotId } },
          signal,
        })
        .then(unwrap),
    staleTime: Infinity,
  })
}
