import { useEffect } from "react"
import {
  queryOptions,
  skipToken,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { grafanaKeys, repairKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

import type { ConnectionInput } from "./connection-problems"

export type GrafanaConnection = components["schemas"]["GrafanaConnection"]
export type GrafanaFolder = components["schemas"]["GrafanaFolder"]
export type GrafanaImport = components["schemas"]["GrafanaImport"]
export type StartImportBody = components["schemas"]["StartGrafanaImportRequest"]

const syncPollMs = 3000

export function connectionsQuery() {
  return queryOptions({
    queryKey: grafanaKeys.connections(),
    queryFn: ({ signal }) =>
      api
        .GET("/grafana", { signal })
        .then(unwrap)
        .then((body) => body.connections),
  })
}

export function useCreateConnection() {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    networkMode: "always",
    mutationFn: (input: ConnectionInput) =>
      api.POST("/grafana", { body: input }).then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: grafanaKeys.connections() }),
  })
}

export function useTestConnection() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (id: string) =>
      api.POST("/grafana/{id}/test", { params: { path: { id } } }).then(unwrap),
    onSuccess: (tested) =>
      queryClient.setQueryData<GrafanaConnection[]>(
        grafanaKeys.connections(),
        (list) =>
          list?.map((connection) =>
            connection.id === tested.id ? tested : connection
          )
      ),
  })
}

export function useDeleteConnection() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (id: string) =>
      api.DELETE("/grafana/{id}", { params: { path: { id } } }).then(unwrap),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: grafanaKeys.connections() }),
  })
}

export function grafanaFoldersQuery(connectionId: string | null) {
  return queryOptions({
    queryKey: grafanaKeys.folders(connectionId ?? ""),
    queryFn:
      connectionId === null
        ? skipToken
        : ({ signal }) =>
            api
              .GET("/grafana/{id}/folders", {
                params: { path: { id: connectionId } },
                signal,
              })
              .then(unwrap),
    retry: false,
  })
}

export function grafanaDatasourcesQuery(connectionId: string | null) {
  return queryOptions({
    queryKey: grafanaKeys.datasources(connectionId ?? ""),
    queryFn:
      connectionId === null
        ? skipToken
        : ({ signal }) =>
            api
              .GET("/grafana/{id}/datasources", {
                params: { path: { id: connectionId } },
                signal,
              })
              .then(unwrap),
    retry: false,
  })
}

export function useStartImport() {
  return useMutation({
    networkMode: "always",
    mutationFn: ({
      connectionId,
      body,
    }: {
      connectionId: string
      body: StartImportBody
    }) =>
      api
        .POST("/grafana/{id}/imports", {
          params: { path: { id: connectionId } },
          body,
        })
        .then(unwrap),
  })
}

export function useImportSync(connectionId: string, syncId: string) {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: grafanaKeys.sync(connectionId, syncId),
    queryFn: ({ signal }) =>
      api
        .GET("/grafana/{id}/imports/{sync_id}", {
          params: { path: { id: connectionId, sync_id: syncId } },
          signal,
        })
        .then(unwrap),
    retry: false,
    refetchInterval: (current) =>
      current.state.data?.status === "running" ? syncPollMs : false,
  })
  const finished = query.data !== undefined && query.data.status !== "running"
  useEffect(() => {
    if (!finished) return
    void queryClient.invalidateQueries({ queryKey: grafanaKeys.connections() })
    void queryClient.invalidateQueries({ queryKey: repairKeys.all })
  }, [finished, queryClient])
  return query
}
