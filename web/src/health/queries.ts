import {
  infiniteQueryOptions,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { healthKeys } from "@/api/keys"

import {
  pollHealth,
  type HealthItem,
  type HealthKind,
  type RunRequest,
} from "./health-labels"

const pageSize = 50

export function healthQuery(kind: HealthKind | null, folderId: string | null) {
  return infiniteQueryOptions({
    queryKey: healthKeys.list(kind, folderId),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/health", {
          params: {
            query: {
              limit: pageSize,
              cursor: pageParam,
              ...(kind === null ? {} : { kind }),
              ...(folderId === null ? {} : { folder_id: folderId }),
            },
          },
          signal,
        })
        .then(unwrap),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  })
}

const pollMs = 5000

export function dashboardsHealthQuery(ids: readonly string[]) {
  return queryOptions({
    queryKey: healthKeys.dashboards(ids),
    queryFn: ({ signal }) =>
      api
        .GET("/health", {
          params: { query: { dashboard_id: [...ids], limit: ids.length } },
          signal,
        })
        .then(unwrap),
    select: (list): ReadonlyMap<string, HealthItem> =>
      new Map(list.dashboards.map((item) => [item.id, item])),
    enabled: ids.length > 0,
    refetchInterval: (query) =>
      pollHealth(
        query.state.data?.job,
        query.state.data?.dashboards ?? [],
        null,
        Date.now()
      )
        ? pollMs
        : false,
  })
}

export function healthSummaryQuery(
  folderId: string | null,
  run: RunRequest | null = null
) {
  return queryOptions({
    queryKey: healthKeys.summary(folderId),
    queryFn: ({ signal }) =>
      api
        .GET("/health", {
          params: {
            query: {
              limit: 1,
              ...(folderId === null ? {} : { folder_id: folderId }),
            },
          },
          signal,
        })
        .then(unwrap),
    select: (list) => ({ summary: list.summary, job: list.job }),
    refetchInterval: (query) =>
      pollHealth(
        query.state.data?.job,
        query.state.data?.dashboards ?? [],
        run,
        Date.now()
      )
        ? pollMs
        : false,
  })
}

export function useRequestHealthRepair() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (dashboardId: string) =>
      api
        .POST("/health/repairs", { body: { dashboard_ids: [dashboardId] } })
        .then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: healthKeys.all }),
  })
}

export function useRunHealth() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: () => api.POST("/health/run").then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: healthKeys.all }),
  })
}
