import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { annotationKeys, dashboardKeys, findingKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { panelRequestLimiter } from "@/dashboards/limiter"
import { asDashboardSpec } from "@/dashboards/queries"
import { panelRetry } from "@/dashboards/use-panel-data"
import type { Panel } from "@/spec"

export type Finding = components["schemas"]["Finding"]
export type FindingEnvelope = components["schemas"]["FindingEnvelope"]

const findingsPage = 50
export const folderFindingsPage = 100

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
      api
        .GET("/findings", {
          params: {
            query: { dashboard_id: dashboardId, limit: findingsPage },
          },
          signal,
        })
        .then(unwrap)
        .then((body) => body.findings),
    staleTime: 10_000,
  })
}

export function folderFindingsQuery(folderId: string) {
  return queryOptions({
    queryKey: findingKeys.inFolder(folderId),
    queryFn: ({ signal }) =>
      api
        .GET("/findings", {
          params: { query: { folder_id: folderId, limit: folderFindingsPage } },
          signal,
        })
        .then(unwrap),
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
  maxDataPoints?: number
  enabled: boolean
}) {
  return useQuery({
    queryKey: findingKeys.panelData(findingId, panel.id, {
      from,
      to,
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

export function usePinFinding({
  id,
  dashboard_id: dashboardId,
}: Pick<Finding, "id" | "dashboard_id">) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      api.POST("/findings/{id}/pin", { params: { path: { id } } }).then(unwrap),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: findingKeys.one(id) })
      void queryClient.invalidateQueries({ queryKey: annotationKeys.all })
      if (dashboardId !== null) {
        void queryClient.invalidateQueries({
          queryKey: findingKeys.forDashboard(dashboardId),
        })
      }
    },
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
    },
  })
}

export function useMoveFinding(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (folderId: string) =>
      api
        .POST("/findings/{id}/move", {
          params: { path: { id } },
          body: { folder_id: folderId },
        })
        .then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: findingKeys.all }),
  })
}
