import { useQuery } from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, isRetryable, unwrap } from "@/api/errors"
import type { Selections } from "@/api/keys"
import { instantPanelTypes, isDataPanel, type Panel } from "@/spec"

import { requestZone } from "@/lib/time"

import { panelRequestLimiter } from "./limiter"
import {
  keepsPreviousData,
  panelDataKey,
  panelFingerprint,
  type PanelSource,
} from "./panel-source"
import { withVariables } from "./search"
import { sortSelections } from "./variables"

type Args = {
  source: PanelSource
  panel: Panel
  from?: string
  to?: string
  timezone?: string
  vars: Selections
  maxDataPoints?: number
  refreshMs: number | null
  enabled: boolean
}

export function maxDataPointsFor(
  panel: Panel,
  pixelWidth: number
): number | undefined {
  if (!isDataPanel(panel) || instantPanelTypes.includes(panel.type)) {
    return undefined
  }
  const bucketed = Math.ceil(pixelWidth / 100) * 100
  return Math.min(Math.max(bucketed, 100), 1100)
}

export function panelRetry(count: number, error: unknown): boolean {
  if (error instanceof ApiError) {
    if (error.code === "query_timeout") return false
    if (error.code === "datasource_unavailable") return count < 1
  }
  return count < 3 && isRetryable(error)
}

export function usePanelData({
  source,
  panel,
  from,
  to,
  timezone,
  vars,
  maxDataPoints,
  refreshMs,
  enabled,
}: Args) {
  const selections = sortSelections(vars)
  const fingerprint = source.kind === "build" ? panelFingerprint(panel) : ""
  return useQuery({
    queryKey: panelDataKey(source, panel.id, fingerprint, {
      from,
      to,
      selections,
      maxDataPoints,
    }),
    queryFn: ({ signal }) => {
      const query: {
        from?: string
        to?: string
        tz?: string
        max_data_points?: number
      } = {}
      if (from !== undefined) query.from = from
      if (to !== undefined) query.to = to
      const tz = requestZone(timezone)
      if (tz !== undefined) query.tz = tz
      if (maxDataPoints !== undefined) query.max_data_points = maxDataPoints
      const params = withVariables(query, selections)
      return panelRequestLimiter.run(
        () =>
          (source.kind === "version"
            ? api.GET(
                "/dashboards/{id}/versions/{version}/panels/{panel_id}/data",
                {
                  params: {
                    path: {
                      id: source.dashboardId,
                      version: source.version,
                      panel_id: panel.id,
                    },
                    query: params,
                  },
                  signal,
                }
              )
            : api.GET("/observe/{id}/build/panels/{panel_id}/data", {
                params: {
                  path: { id: source.requestId, panel_id: panel.id },
                  query: params,
                },
                signal,
              })
          ).then(unwrap),
        signal
      )
    },
    enabled,
    placeholderData: (previous, previousQuery) =>
      previousQuery !== undefined &&
      keepsPreviousData(source, fingerprint, previousQuery.queryKey)
        ? previous
        : undefined,
    staleTime: 0,
    gcTime: 300_000,
    refetchInterval: refreshMs ?? false,
    retry: panelRetry,
  })
}
