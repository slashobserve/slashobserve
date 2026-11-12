import { queryOptions, useQuery } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { observationKeys } from "@/api/keys"
import { panelRequestLimiter } from "@/dashboards/limiter"
import { panelRetry } from "@/dashboards/use-panel-data"
import { formatDate } from "@/lib/date-format"
import type { Block } from "@/observations/queries"
import type { DataPanel, Panel } from "@/spec"

export const blockPanelId = "block"

export type PanelBlockBody = { panel: Panel; from?: string; to?: string }
export type RunBlockBody = { runId: string; cites: string[] }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function optionalString(value: unknown): string | undefined | null {
  if (value === undefined) return undefined
  return typeof value === "string" ? value : null
}

export function panelBlockBody(block: Block): PanelBlockBody | null {
  const { panel, from, to } = block.body
  if (!isRecord(panel) || typeof panel.type !== "string") return null
  const parsedFrom = optionalString(from)
  const parsedTo = optionalString(to)
  if (parsedFrom === null || parsedTo === null) return null
  const body: PanelBlockBody = {
    panel: { ...panel, id: blockPanelId } as unknown as Panel,
  }
  if (parsedFrom !== undefined) body.from = parsedFrom
  if (parsedTo !== undefined) body.to = parsedTo
  return body
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
  return `Frozen, ${start} to ${end}`
}

export function useBlockPanelData({
  blockId,
  from,
  to,
  maxDataPoints,
  enabled,
}: {
  blockId: string
  from?: string
  to?: string
  maxDataPoints?: number
  enabled: boolean
}) {
  return useQuery({
    queryKey: observationKeys.blockData(blockId, {
      from,
      to,
      maxDataPoints,
      selections: {},
    }),
    queryFn: ({ signal }) => {
      const query: { from?: string; to?: string; max_data_points?: number } = {}
      if (from !== undefined) query.from = from
      if (to !== undefined) query.to = to
      if (maxDataPoints !== undefined) query.max_data_points = maxDataPoints
      return panelRequestLimiter.run(
        () =>
          api
            .GET("/blocks/{id}/data", {
              params: { path: { id: blockId }, query },
              signal,
            })
            .then(unwrap),
        signal
      )
    },
    enabled,
    retry: panelRetry,
    staleTime: 0,
  })
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
