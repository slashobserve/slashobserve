import {
  queryOptions,
  useMutation,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import {
  dashboardKeys,
  datasourceKeys,
  folderKeys,
  importKeys,
  observeKeys,
  proposalKeys,
  userKeys,
} from "@/api/keys"
import type { paths } from "@/api/schema.gen"
import type { Dashboard } from "@/spec"

import type { SaveOutcome } from "./save-outcome"

export type DashboardEnvelope =
  paths["/dashboards/{id}"]["get"]["responses"][200]["content"]["application/json"]
export type PanelDataResponse =
  paths["/dashboards/{id}/versions/{version}/panels/{panel_id}/data"]["get"]["responses"][200]["content"]["application/json"]
export type WireFrame = PanelDataResponse["frames"][number]
export type PanelDataMeta = PanelDataResponse["meta"]

export function asDashboardSpec(raw: unknown): Dashboard {
  if (!isDashboardShape(raw)) throw new Error("Server returned an invalid Spec")
  return raw
}

function isDashboardShape(raw: unknown): raw is Dashboard {
  return (
    typeof raw === "object" &&
    raw !== null &&
    (raw as { specVersion?: unknown }).specVersion === "0" &&
    Array.isArray((raw as { panels?: unknown }).panels)
  )
}

export function latestDashboardQuery(queryClient: QueryClient, id: string) {
  return queryOptions({
    queryKey: dashboardKeys.latest(id),
    // The route loader awaits this fetch, so it must not read the query's abort signal: React Query cancels a fetch that read it when the last observer unmounts, and the loader would reject with CancelledError.
    queryFn: async () => {
      const body = await api
        .GET("/dashboards/{id}", { params: { path: { id } } })
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

export function useMoveDashboard() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (input: { id: string; folderId: string }) =>
      api
        .PATCH("/dashboards/{id}", {
          params: { path: { id: input.id } },
          body: { folder_id: input.folderId },
        })
        .then(unwrap),
    onSettled: (_data, _error, input) =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: dashboardKeys.list() }),
        queryClient.invalidateQueries({
          queryKey: dashboardKeys.latest(input.id),
        }),
        queryClient.invalidateQueries({ queryKey: folderKeys.all }),
      ]),
  })
}

export function useDuplicateDashboard(sourceId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (input: { title: string; folderId: string }) =>
      api
        .POST("/dashboards/{id}/duplicate", {
          params: { path: { id: sourceId } },
          body: { title: input.title, folder_id: input.folderId },
        })
        .then(unwrap),
    onSettled: () =>
      Promise.all(
        [
          dashboardKeys.list(),
          dashboardKeys.titles(),
          dashboardKeys.tags(),
          folderKeys.all,
        ].map((queryKey) => queryClient.invalidateQueries({ queryKey }))
      ),
  })
}

export function useDeleteDashboard(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationKey: dashboardKeys.deletes(id),
    mutationFn: () =>
      api.DELETE("/dashboards/{id}", { params: { path: { id } } }).then(unwrap),
    onSettled: () =>
      Promise.all(
        [
          dashboardKeys.list(),
          dashboardKeys.titles(),
          dashboardKeys.tags(),
          folderKeys.all,
          observeKeys.mineAll(),
          importKeys.all,
          datasourceKeys.all,
          userKeys.all,
        ].map((queryKey) => queryClient.invalidateQueries({ queryKey }))
      ),
  })
}

export function forgetDashboard(queryClient: QueryClient, id: string) {
  queryClient.removeQueries({ queryKey: dashboardKeys.one(id) })
}

export function useAddVersion(id: string) {
  const queryClient = useQueryClient()
  const store = useStoreSavedVersion(id)
  return useMutation({
    mutationKey: dashboardKeys.saves(id),
    mutationFn: (input: {
      spec: Dashboard
      summary: string
      parent_version: number
      propose: boolean
    }): Promise<SaveOutcome> =>
      api
        .POST("/dashboards/{id}/versions", {
          params: { path: { id } },
          body: {
            spec: { ...input.spec },
            summary: input.summary,
            parent_version: input.parent_version,
            merge: "auto",
            ...(input.propose ? { propose_if_needed: true } : {}),
          },
        })
        .then((result) => {
          const body = unwrap(result)
          if ("proposal_id" in body) {
            return {
              kind: "proposed",
              proposalId: body.proposal_id,
              revision: body.revision,
              opened: body.opened,
            }
          }
          return {
            kind: "saved",
            envelope: body,
            version: body.version.version,
            mergedFrom: body.merged_from ?? null,
          }
        }),
    onSuccess: async (outcome) => {
      if (outcome.kind === "saved") return store(outcome.envelope)
      await queryClient.invalidateQueries({
        queryKey: proposalKeys.list(id, "open"),
      })
    },
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
