import { createContext, useContext } from "react"
import { useQuery, type UseQueryResult } from "@tanstack/react-query"

import type { InputMode } from "@/ask/input-mode"
import type { PanelDataResponse } from "@/dashboards/queries"
import type { TimeRangeValue } from "@/dashboards/time-range-picker"
import type { DatasourceStatus } from "@/datasources/queries"
import { exploreQuery } from "@/logs/explore"

import type { MetricsMode, MetricsSearch } from "./search"

const minPoints = 100
const maxPoints = 1100

function pointsFor(width: number): number {
  const bucketed = Math.ceil(width / 100) * 100
  return Math.min(Math.max(bucketed, minPoints), maxPoints)
}

export function useMetricsResults({
  uid,
  query,
  range,
  mode,
  width,
}: {
  uid: string
  query: string | undefined
  range: TimeRangeValue
  mode: MetricsMode
  width: number
}): UseQueryResult<PanelDataResponse> {
  return useQuery({
    ...exploreQuery(uid, {
      query: query ?? "",
      kind: mode === "table" ? "instant" : "range",
      from: range.from,
      to: range.to,
      ...(mode === "graph" && { maxDataPoints: pointsFor(width) }),
    }),
    enabled: query !== undefined && (mode === "table" || width > 0),
  })
}

export type MetricsWorkspace = {
  datasource: DatasourceStatus
  search: MetricsSearch
  mode: MetricsMode
  inputMode: InputMode
  setInputMode: (next: InputMode, remember?: boolean) => void
  effectiveQuery: string | undefined
  update: (
    next: Partial<MetricsSearch>,
    options?: { replace?: boolean }
  ) => void
  draft: string
  setDraft: (text: string) => void
  run: (text?: string, asked?: string) => void
  range: TimeRangeValue
  focusQuery: () => void
  registerQueryFocus: (focus: () => void) => void
}

export const MetricsWorkspaceContext = createContext<MetricsWorkspace | null>(
  null
)

export function useMetricsWorkspace(): MetricsWorkspace {
  const workspace = useContext(MetricsWorkspaceContext)
  if (workspace === null) {
    throw new Error(
      "useMetricsWorkspace needs a MetricsWorkspaceContext provider"
    )
  }
  return workspace
}
