import {
  queryOptions,
  useMutation,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import type { paths } from "@/api/schema.gen"
import { validateSpecSchema, type Dashboard } from "@/spec"

export type DashboardEnvelope =
  paths["/dashboards/{id}"]["get"]["responses"][200]["content"]["application/json"]
export type PanelDataResponse =
  paths["/dashboards/{id}/versions/{version}/panels/{panel_id}/data"]["get"]["responses"][200]["content"]["application/json"]
export type WireFrame = PanelDataResponse["frames"][number]
export type PanelDataMeta = PanelDataResponse["meta"]

export function asDashboardSpec(raw: unknown): Dashboard {
  const result = validateSpecSchema(raw)
  if (!result.valid) throw new Error("Server returned an invalid Spec")
  return result.spec
}

export function latestDashboardQuery(queryClient: QueryClient, id: string) {
  return queryOptions({
    queryKey: dashboardKeys.latest(id),
    queryFn: async ({ signal }) => {
      const body = await api
        .GET("/dashboards/{id}", { params: { path: { id } }, signal })
        .then(unwrap)
      queryClient.setQueryData(
        dashboardKeys.version(id, body.version.version),
        body
      )
      return body
    },
    staleTime: 10_000,
  })
}

export function dashboardVersionQuery(id: string, version: number) {
  return queryOptions({
    queryKey: dashboardKeys.version(id, version),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards/{id}/versions/{version}", {
          params: { path: { id, version } },
          signal,
        })
        .then(unwrap),
    staleTime: Infinity,
  })
}

function useStoreSavedVersion(id: string) {
  const queryClient = useQueryClient()
  return async (body: DashboardEnvelope) => {
    await queryClient.cancelQueries({ queryKey: dashboardKeys.latest(id) })
    queryClient.setQueryData(dashboardKeys.latest(id), body)
    queryClient.setQueryData(
      dashboardKeys.version(id, body.version.version),
      body
    )
    void queryClient.invalidateQueries({ queryKey: dashboardKeys.history(id) })
    void queryClient.invalidateQueries({ queryKey: dashboardKeys.list() })
  }
}

export function useAddVersion(id: string) {
  const store = useStoreSavedVersion(id)
  return useMutation({
    mutationKey: dashboardKeys.saves(id),
    mutationFn: (input: {
      spec: Dashboard
      summary: string
      parent_version: number
    }) =>
      api
        .POST("/dashboards/{id}/versions", {
          params: { path: { id } },
          body: {
            spec: { ...input.spec },
            summary: input.summary,
            parent_version: input.parent_version,
          },
        })
        .then(unwrap),
    onSuccess: store,
  })
}

export function useRestoreVersion(id: string) {
  const store = useStoreSavedVersion(id)
  return useMutation({
    mutationKey: dashboardKeys.saves(id),
    mutationFn: (input: {
      version: number
      parent_version: number
      summary?: string
    }) =>
      api
        .POST("/dashboards/{id}/versions/{version}/restore", {
          params: { path: { id, version: input.version } },
          body: {
            parent_version: input.parent_version,
            summary: input.summary,
          },
        })
        .then(unwrap),
    onSuccess: store,
  })
}
