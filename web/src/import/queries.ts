import {
  queryOptions,
  skipToken,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { dashboardKeys, importKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

import { toImportError } from "./errors"

export type Source = { id: number; name: string; text: string; pasted: boolean }
export type Mapping = Record<string, string>
type ImportPreview = components["schemas"]["ImportPreview"]
export type ImportedDashboard = components["schemas"]["ImportedDashboard"]

let lastSourceId = 0

export function newSource(name: string, text: string, pasted: boolean): Source {
  lastSourceId += 1
  return { id: lastSourceId, name, text, pasted }
}

function requestBody(source: Source, mapping: Mapping) {
  return Object.keys(mapping).length === 0
    ? { json: source.text }
    : { json: source.text, mapping }
}

export function previewQuery(source: Source | null, mapping: Mapping) {
  return queryOptions({
    queryKey: importKeys.preview(source?.id ?? 0, mapping),
    queryFn: source
      ? ({ signal }) =>
          api
            .POST("/imports/preview", {
              body: requestBody(source, mapping),
              signal,
            })
            .then((result) => {
              if (!result.response.ok) {
                throw toImportError(result.response, result.error)
              }
              return result.data as ImportPreview
            })
      : skipToken,
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[2] === source?.id ? previous : undefined,
  })
}

export function useImportDashboard() {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    networkMode: "always",
    mutationFn: async ({
      source,
      mapping,
    }: {
      source: Source
      mapping: Mapping
    }): Promise<ImportedDashboard> => {
      const result = await api.POST("/imports", {
        body: requestBody(source, mapping),
      })
      if (!result.response.ok) {
        throw toImportError(result.response, result.error)
      }
      const data = unwrap(result)
      if (!("import" in data)) {
        throw new ApiError(
          result.response.status,
          "unexpected_response",
          "The import did not return a report."
        )
      }
      return data
    },
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: dashboardKeys.list() }),
        queryClient.invalidateQueries({ queryKey: importKeys.list("mine") }),
      ]),
  })
}
