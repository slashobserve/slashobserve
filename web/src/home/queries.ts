import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { homeKeys, starKeys, type HomeSearch } from "@/api/keys"

import { withStar, type Star, type StarTarget } from "./stars"

const firstDashboardsLimit = 8
const searchLimit = 20

export function useStars() {
  return useQuery({
    queryKey: starKeys.list(),
    queryFn: ({ signal }) =>
      api
        .GET("/stars", { signal })
        .then(unwrap)
        .then((body) => body.items),
    staleTime: 15_000,
  })
}

export function useStarMutation() {
  const queryClient = useQueryClient()
  const key = starKeys.list()
  return useMutation({
    mutationFn: ({
      target,
      starred,
    }: {
      target: StarTarget
      starred: boolean
    }) => {
      const params = { params: { path: { id: target.id } } }
      return (
        starred
          ? api.PUT("/dashboards/{id}/star", params)
          : api.DELETE("/dashboards/{id}/star", params)
      ).then(unwrap)
    },
    onMutate: async ({ target, starred }) => {
      await queryClient.cancelQueries({ queryKey: key })
      const previous = queryClient.getQueryData<Star[]>(key)
      queryClient.setQueryData<Star[]>(key, (items) =>
        withStar(items ?? [], target, starred, new Date())
      )
      return { previous }
    },
    onError: (_error, { starred }, context) => {
      queryClient.setQueryData(key, context?.previous)
      toast.error(
        starred ? "Could not star the dashboard" : "Could not remove the star"
      )
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: key }),
  })
}

export function useFirstDashboards(enabled: boolean) {
  return useQuery({
    queryKey: homeKeys.firstDashboards(),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards", {
          params: { query: { order: "folder", limit: firstDashboardsLimit } },
          signal,
        })
        .then(unwrap)
        .then((body) => body.dashboards),
    enabled,
    staleTime: 15_000,
  })
}

export function useDashboardSearch(search: HomeSearch, enabled: boolean) {
  return useQuery({
    queryKey: homeKeys.search(search),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards", {
          params: {
            query: {
              q: search.q,
              tag: search.tag.length > 0 ? search.tag : undefined,
              limit: searchLimit,
            },
          },
          signal,
        })
        .then(unwrap)
        .then((body) => body.dashboards),
    enabled,
    staleTime: 15_000,
    placeholderData: (previous) => previous,
  })
}
