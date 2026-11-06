import {
  infiniteQueryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { repairKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

import { anyActive } from "./repair-state"

export type RepairsQueued = components["schemas"]["RepairsQueued"]
type RepairTarget =
  { dashboardId: string } | { all: true; folderId: string | null }

const pageSize = 50
const pollMs = 5000

export function repairsQuery(folderId: string | null) {
  return infiniteQueryOptions({
    queryKey: repairKeys.list(folderId),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/repairs", {
          params: {
            query: {
              limit: pageSize,
              cursor: pageParam,
              ...(folderId === null ? {} : { folder_id: folderId }),
            },
          },
          signal,
        })
        .then(unwrap),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    refetchInterval: (query) =>
      anyActive(
        query.state.data?.pages.flatMap((page) => page.dashboards) ?? []
      )
        ? pollMs
        : false,
  })
}

function bodyOf(target: RepairTarget) {
  if ("dashboardId" in target) return { dashboard_ids: [target.dashboardId] }
  return {
    all: true as const,
    ...(target.folderId === null ? {} : { folder_id: target.folderId }),
  }
}

export function useRequestRepairs() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (target: RepairTarget) =>
      api.POST("/repairs", { body: bodyOf(target) }).then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: repairKeys.all }),
  })
}
