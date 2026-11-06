import {
  infiniteQueryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { healthKeys } from "@/api/keys"

import type { HealthKind } from "./health-labels"

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
