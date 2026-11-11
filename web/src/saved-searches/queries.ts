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

const listPage = 100

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
              limit: listPage,
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
