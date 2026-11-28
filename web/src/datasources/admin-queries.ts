import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, toApiError } from "@/api/errors"
import { datasourceKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"

import type { DatasourceDraft } from "./draft"

export type DatasourceUser =
  components["schemas"]["DatasourceInUseError"]["used_by"][number]

export type HeldItem = NonNullable<
  components["schemas"]["RestrictedContentError"]["items"]
>[number]

export class DatasourceError extends ApiError {
  readonly fields: { field: string; message: string }[]
  readonly usedBy: DatasourceUser[]
  readonly held: HeldItem[]
  readonly unlisted: number
  readonly hidden: number

  constructor(base: ApiError, body: unknown) {
    super(base.status, base.code, base.message)
    const record =
      typeof body === "object" && body !== null
        ? (body as Record<string, unknown>)
        : {}
    this.fields = Array.isArray(record.fields)
      ? (record.fields as { field: string; message: string }[])
      : []
    this.usedBy = Array.isArray(record.used_by)
      ? (record.used_by as DatasourceUser[])
      : []
    this.held = Array.isArray(record.items) ? (record.items as HeldItem[]) : []
    this.unlisted = typeof record.unlisted === "number" ? record.unlisted : 0
    this.hidden = typeof record.hidden === "number" ? record.hidden : 0
  }
}

function settle<T>(result: {
  data?: T
  error?: unknown
  response: Response
}): T {
  if (!result.response.ok || result.error !== undefined) {
    throw new DatasourceError(
      toApiError(result.response, result.error),
      result.error
    )
  }
  return result.data as T
}

export function datasourceConfigQuery(uid: string) {
  return queryOptions({
    queryKey: datasourceKeys.config(uid),
    queryFn: ({ signal }) =>
      api
        .GET("/datasources/{uid}/config", {
          params: { path: { uid } },
          signal,
        })
        .then(settle),
    gcTime: 0,
  })
}

export function useCreateDatasource() {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    mutationFn: (draft: DatasourceDraft) =>
      api.POST("/datasources", { body: draft }).then(settle),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: datasourceKeys.list() }),
  })
}

export function useUpdateDatasource(uid: string) {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    mutationFn: (draft: DatasourceDraft) =>
      api
        .PATCH("/datasources/{uid}", {
          params: { path: { uid } },
          body: draft,
        })
        .then(settle),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: datasourceKeys.all }),
  })
}

export function useDeleteDatasource() {
  const queryClient = useQueryClient()
  return useMutation({
    gcTime: 0,
    mutationFn: (uid: string) =>
      api
        .DELETE("/datasources/{uid}", { params: { path: { uid } } })
        .then(settle),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: datasourceKeys.all }),
  })
}

export function useTestDatasource() {
  return useMutation({
    gcTime: 0,
    mutationFn: ({
      draft,
      base,
    }: {
      draft: DatasourceDraft
      base?: string | undefined
    }) =>
      api
        .POST("/datasources/test", {
          params: { query: base === undefined ? {} : { base } },
          body: draft,
        })
        .then(settle),
  })
}
