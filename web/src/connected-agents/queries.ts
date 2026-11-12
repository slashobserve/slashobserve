import {
  infiniteQueryOptions,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query"
import { toast } from "sonner"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { grantKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { sentence } from "@/observe/queries"

import type { ConnectedAgent, GrantMode, GrantScope } from "./rows"

type GrantPage = components["schemas"]["ConnectedAgentList"]

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

export function useSetGrantMode() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (input: { id: string; mode: GrantMode }) =>
      api
        .PATCH("/me/grants/{id}", {
          params: { path: { id: input.id } },
          body: { mode: input.mode },
        })
        .then(unwrap),
    onSuccess: (grant: ConnectedAgent) =>
      queryClient.setQueryData<InfiniteData<GrantPage>>(
        grantKeys.list("mine"),
        (data) =>
          data === undefined
            ? data
            : {
                ...data,
                pages: data.pages.map((page) => ({
                  ...page,
                  grants: page.grants.map((item) =>
                    item.id === grant.id ? grant : item
                  ),
                })),
              }
      ),
    onError: (error) =>
      toast.error(
        error instanceof ApiError
          ? sentence(error.message)
          : "Could not change the mode"
      ),
    onSettled: () => queryClient.invalidateQueries({ queryKey: grantKeys.all }),
  })
}
