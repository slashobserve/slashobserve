import {
  queryOptions,
  type QueryClient,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { observationKeys, rcaKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { panelRequestLimiter } from "@/dashboards/limiter"
import { panelRetry } from "@/dashboards/use-panel-data"

export type RcaSummary = components["schemas"]["ObservationRCASummary"]
export type RcaLink = components["schemas"]["RCAPublicLink"]
export type PublicRca = components["schemas"]["PublicRCA"]
export type RcaBlock = components["schemas"]["RCABlock"]

export function rcasQuery(observationId: string) {
  return queryOptions({
    queryKey: observationKeys.rcas(observationId),
    queryFn: ({ signal }) =>
      api
        .GET("/observations/{id}/rcas", {
          params: { path: { id: observationId } },
          signal,
        })
        .then(unwrap),
    select: (data) => data.rcas,
  })
}

export function rcaQuery(observationId: string, revision: number) {
  return queryOptions({
    queryKey: observationKeys.rca(observationId, revision),
    queryFn: ({ signal }) =>
      api
        .GET("/observations/{id}/rcas/{revision}", {
          params: { path: { id: observationId, revision } },
          signal,
        })
        .then(unwrap),
    select: (data) => data.rca,
    staleTime: Infinity,
  })
}

export function rcaLinksQuery(observationId: string, revision: number) {
  return queryOptions({
    queryKey: observationKeys.rcaLinks(observationId, revision),
    queryFn: ({ signal }) =>
      api
        .GET("/observations/{id}/rcas/{revision}/public-links", {
          params: { path: { id: observationId, revision } },
          signal,
        })
        .then(unwrap),
    select: (data) => data.public_links,
  })
}

export function usePublishRca(observationId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      api
        .POST("/observations/{id}/rcas", {
          params: { path: { id: observationId } },
        })
        .then(unwrap),
    onSettled: () =>
      Promise.all([
        queryClient.invalidateQueries({
          queryKey: observationKeys.rcas(observationId),
          exact: true,
        }),
        queryClient.invalidateQueries({
          queryKey: observationKeys.allChanges(observationId),
        }),
      ]),
  })
}

export function useWithdrawRca(observationId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (revision: number) =>
      api
        .DELETE("/observations/{id}/rcas/{revision}", {
          params: { path: { id: observationId, revision } },
        })
        .then(unwrap),
    onSuccess: (_, revision) => {
      queryClient.removeQueries({
        queryKey: observationKeys.rca(observationId, revision),
      })
    },
    onSettled: () =>
      Promise.all([
        queryClient.invalidateQueries({
          queryKey: observationKeys.rcas(observationId),
          exact: true,
        }),
        queryClient.invalidateQueries({
          queryKey: observationKeys.allChanges(observationId),
        }),
      ]),
  })
}

function refreshLinks(
  queryClient: QueryClient,
  observationId: string,
  revision: number
) {
  return Promise.all([
    queryClient.invalidateQueries({
      queryKey: observationKeys.rcaLinks(observationId, revision),
    }),
    queryClient.invalidateQueries({
      queryKey: observationKeys.rcas(observationId),
      exact: true,
    }),
  ])
}

export function useCreateRcaLink(observationId: string, revision: number) {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    mutationFn: (body: components["schemas"]["CreateRCAPublicLinkRequest"]) =>
      api
        .POST("/observations/{id}/rcas/{revision}/public-links", {
          params: { path: { id: observationId, revision } },
          body,
        })
        .then(unwrap),
    onSuccess: () => refreshLinks(queryClient, observationId, revision),
  })
}

export function useRevokeRcaLink(observationId: string, revision: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (linkId: string) =>
      api
        .DELETE("/observations/{id}/public-links/{link_id}", {
          params: { path: { id: observationId, link_id: linkId } },
        })
        .then(unwrap),
    onSettled: () => refreshLinks(queryClient, observationId, revision),
  })
}

function retryUnlessGone(count: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status === 404) return false
  return panelRetry(count, error)
}

export function publicRcaQuery(token: string) {
  return queryOptions({
    queryKey: rcaKeys.publicView(token),
    queryFn: ({ signal }) =>
      api
        .GET("/public/{token}/rca", { params: { path: { token } }, signal })
        .then(unwrap),
    staleTime: Infinity,
    retry: retryUnlessGone,
  })
}

export function publicRcaSnapshotQuery(token: string, snapshotId: string) {
  return queryOptions({
    queryKey: rcaKeys.publicSnapshot(token, snapshotId),
    queryFn: ({ signal }) =>
      panelRequestLimiter.run(
        () =>
          api
            .GET("/public/{token}/rca/snapshots/{snapshot_id}", {
              params: { path: { token, snapshot_id: snapshotId } },
              signal,
            })
            .then(unwrap),
        signal
      ),
    staleTime: Infinity,
    retry: retryUnlessGone,
  })
}
