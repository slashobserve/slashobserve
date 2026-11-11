import {
  infiniteQueryOptions,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { grantKeys } from "@/api/keys"

import type { GrantScope } from "./rows"

function grantsQuery(scope: GrantScope) {
  return infiniteQueryOptions({
    queryKey: grantKeys.list(scope),
    queryFn: ({ pageParam, signal }) => {
      const request = { params: { query: { cursor: pageParam } }, signal }
      return (
        scope === "mine"
          ? api.GET("/me/grants", request)
          : api.GET("/grants", request)
      ).then(unwrap)
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => data.pages.flatMap((page) => page.grants),
  })
}

export function useMyGrants() {
  return useInfiniteQuery(grantsQuery("mine"))
}

export function useAllGrants() {
  return useInfiniteQuery(grantsQuery("all"))
}

export function useDisconnect(scope: GrantScope) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (id: string) => {
      const params = { params: { path: { id } } }
      return (
        scope === "mine"
          ? api.DELETE("/me/grants/{id}", params)
          : api.DELETE("/grants/{id}", params)
      ).then(unwrap)
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: grantKeys.all }),
  })
}
