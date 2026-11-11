import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { savedSearchKeys, type SavedSearchListParams } from "@/api/keys"
import type { components } from "@/api/schema.gen"

export type SavedSearch = components["schemas"]["SavedSearch"]
export type CreateSavedSearch =
  components["schemas"]["CreateSavedSearchRequest"]

export const savedSearchesPage = 100

export function savedSearchesQuery(params: SavedSearchListParams) {
  return queryOptions({
    queryKey: savedSearchKeys.list(params),
    queryFn: ({ signal }) =>
      api
        .GET("/saved-searches", {
          params: {
            query: {
              kind: params.kind,
              folder_id: params.folderId,
              limit: savedSearchesPage,
            },
          },
          signal,
        })
        .then(unwrap),
    staleTime: 15_000,
  })
}

export function useSaveSearch() {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (body: CreateSavedSearch) =>
      api.POST("/saved-searches", { body }).then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: savedSearchKeys.all }),
  })
}

export function useRenameSavedSearch(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (title: string) =>
      api
        .PATCH("/saved-searches/{id}", {
          params: { path: { id } },
          body: { title },
        })
        .then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: savedSearchKeys.all }),
  })
}

export function useMoveSavedSearch(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: (folderId: string) =>
      api
        .PATCH("/saved-searches/{id}", {
          params: { path: { id } },
          body: { folder_id: folderId },
        })
        .then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: savedSearchKeys.all }),
  })
}

export function useDeleteSavedSearch(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    networkMode: "always",
    mutationFn: () =>
      api
        .DELETE("/saved-searches/{id}", { params: { path: { id } } })
        .then(unwrap),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: savedSearchKeys.all }),
  })
}
