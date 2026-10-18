import { useQuery } from "@tanstack/react-query"

import { api } from "@/api/client"
import { isRetryable, unwrap } from "@/api/errors"
import { dashboardKeys, type Selections } from "@/api/keys"
import type { Panel } from "@/spec"

import { panelRequestLimiter } from "./limiter"
import { withVariables } from "./search"
import { sortSelections } from "./variables"

type Args = {
  dashboardId: string
  version: number
  panel: Panel
  from?: string
  to?: string
  vars: Selections
  maxDataPoints?: number
  refreshMs: number | null
  enabled: boolean
}

export function maxDataPointsFor(
  panel: Panel,
  pixelWidth: number
): number | undefined {
  if (panel.type !== "timeseries") return undefined
  const bucketed = Math.ceil(pixelWidth / 100) * 100
  return Math.min(Math.max(bucketed, 100), 1100)
}

export function usePanelData({
  dashboardId,
  version,
  panel,
  from,
  to,
  vars,
  maxDataPoints,
  refreshMs,
  enabled,
}: Args) {
  const selections = sortSelections(vars)
  return useQuery({
    queryKey: dashboardKeys.panelData(dashboardId, version, panel.id, {
      from,
      to,
      selections,
      maxDataPoints,
    }),
    queryFn: ({ signal }) => {
      const query: { from?: string; to?: string; max_data_points?: number } = {}
      if (from !== undefined) query.from = from
      if (to !== undefined) query.to = to
      if (maxDataPoints !== undefined) query.max_data_points = maxDataPoints
      return panelRequestLimiter.run(
        () =>
          api
            .GET("/dashboards/{id}/versions/{version}/panels/{panel_id}/data", {
              params: {
                path: { id: dashboardId, version, panel_id: panel.id },
                query: withVariables(query, selections),
              },
              signal,
            })
            .then(unwrap),
        signal
      )
    },
    enabled,
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[3] === version ? previous : undefined,
    staleTime: 0,
    gcTime: 300_000,
    refetchInterval: refreshMs ?? false,
    retry: (count, error) => count < 3 && isRetryable(error),
  })
}
