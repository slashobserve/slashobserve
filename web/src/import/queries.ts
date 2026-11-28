import {
  queryOptions,
  skipToken,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { dashboardKeys, folderKeys, importKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

import { toImportError } from "./errors"
import { rememberImportFolder } from "./folder-choice"

export type Source = { id: number; name: string; text: string; pasted: boolean }
export type Mapping = Record<string, string>
type ImportPreview = components["schemas"]["ImportPreview"]
export type ImportedDashboard = components["schemas"]["ImportedDashboard"]

let lastSourceId = 0

export function newSource(name: string, text: string, pasted: boolean): Source {
  lastSourceId += 1
  return { id: lastSourceId, name, text, pasted }
}

function requestBody(
  source: Source,
  mapping: Mapping,
  folder?: string,
  update?: string
) {
  return {
    json: source.text,
    ...(Object.keys(mapping).length === 0 ? {} : { mapping }),
    ...(folder === undefined ? {} : { folder }),
    ...(update === undefined ? {} : { update_dashboard: update }),
  }
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
    mutationFn: async ({
      source,
      mapping,
      folder,
      update,
    }: {
      source: Source
      mapping: Mapping
      folder?: string | undefined
      update?: string | undefined
    }): Promise<ImportedDashboard> => {
      const result = await api.POST("/imports", {
        body: requestBody(source, mapping, folder, update),
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
    onSuccess: (data) => {
      rememberImportFolder(data.dashboard.folder)
      return Promise.all([
        queryClient.invalidateQueries({ queryKey: dashboardKeys.list() }),
        queryClient.invalidateQueries({ queryKey: folderKeys.all }),
        queryClient.invalidateQueries({ queryKey: importKeys.list("mine") }),
      ])
    },
  })
}

export type CreatedDashboard = components["schemas"]["DashboardWithVersion"]

export function useCreateDashboardFromSpec() {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    mutationFn: async ({
      spec,
      folderId,
    }: {
      spec: Record<string, unknown>
      folderId?: string | undefined
    }): Promise<CreatedDashboard> =>
      unwrap(
        await api.POST("/dashboards", {
          body: {
            spec,
            ...(folderId === undefined ? {} : { folder_id: folderId }),
          },
        })
      ),
    onSuccess: (data) => {
      rememberImportFolder(data.dashboard.folder)
      return Promise.all([
        queryClient.invalidateQueries({ queryKey: dashboardKeys.list() }),
        queryClient.invalidateQueries({ queryKey: folderKeys.all }),
      ])
    },
  })
}
