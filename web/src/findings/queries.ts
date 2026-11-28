import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { chatKeys, dashboardKeys, findingKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { panelRequestLimiter } from "@/dashboards/limiter"
import { asDashboardSpec } from "@/dashboards/queries"
import { panelRetry } from "@/dashboards/use-panel-data"
import { followCursor } from "@/lib/follow-cursor"
import type { Panel } from "@/spec"

export type Finding = components["schemas"]["Finding"]

const findingsPage = 50

export function findingQuery(id: string) {
  return queryOptions({
    queryKey: findingKeys.one(id),
    queryFn: ({ signal }) =>
      api
        .GET("/findings/{id}", { params: { path: { id } }, signal })
        .then(unwrap),
    staleTime: 60_000,
  })
}

export function findingsForDashboardQuery(dashboardId: string) {
  return queryOptions({
    queryKey: findingKeys.forDashboard(dashboardId),
    queryFn: ({ signal }) =>
      followCursor((cursor) =>
        api
          .GET("/findings", {
            params: {
              query: {
                dashboard_id: dashboardId,
                limit: findingsPage,
                ...(cursor === undefined ? {} : { cursor }),
              },
            },
            signal,
          })
          .then(unwrap)
          .then((body) => ({
            items: body.findings,
            next: body.next_cursor ?? null,
          }))
      ),
    staleTime: 10_000,
  })
}

export function evidencePanels(finding: Finding): Panel[] {
  if (finding.evidence.length === 0) return []
  return asDashboardSpec({
    specVersion: "0",
    title: finding.title,
    time: { from: finding.from, to: finding.to },
    panels: finding.evidence,
  }).panels
}

export function useFindingPanelData({
  findingId,
  panel,
  from,
  to,
  maxDataPoints,
  enabled,
}: {
  findingId: string
  panel: Panel
  from?: string
  to?: string
  maxDataPoints?: number | undefined
  enabled: boolean
}) {
  return useQuery({
    queryKey: findingKeys.panelData(findingId, panel.id, {
      ...(from === undefined ? {} : { from }),
      ...(to === undefined ? {} : { to }),
      ...(maxDataPoints === undefined ? {} : { maxDataPoints }),
    }),
    queryFn: ({ signal }) => {
      const query: { from?: string; to?: string; max_data_points?: number } = {}
      if (from !== undefined) query.from = from
      if (to !== undefined) query.to = to
      if (maxDataPoints !== undefined) query.max_data_points = maxDataPoints
      return panelRequestLimiter.run(
        () =>
          api
            .GET("/findings/{id}/panels/{panel_id}/data", {
              params: {
                path: { id: findingId, panel_id: panel.id },
                query,
              },
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

export function useMakeDashboard(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      api
        .POST("/findings/{id}/dashboard", { params: { path: { id } } })
        .then(unwrap),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: dashboardKeys.list() })
      void queryClient.invalidateQueries({ queryKey: dashboardKeys.titles() })
      void queryClient.invalidateQueries({ queryKey: chatKeys.lists() })
      void queryClient.invalidateQueries({ queryKey: chatKeys.details() })
    },
  })
}
