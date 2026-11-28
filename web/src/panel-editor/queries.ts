import {
  keepPreviousData,
  queryOptions,
  useMutation,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, isRetryable, unwrap, sentence } from "@/api/errors"
import {
  datasourceKeys,
  editorKeys,
  type NameListParams,
  type PreviewKeyParams,
  type Selections,
} from "@/api/keys"
import { panelRequestLimiter } from "@/dashboards/limiter"
import type { PanelDataResponse } from "@/dashboards/queries"
import { stableJson } from "@/lib/json"
import { requestZone } from "@/lib/time"
import { resolveDatasourceRef, type DataPanel } from "@/spec"

import type { EditablePanel } from "./draft"
import { missingPanelMessage } from "./panel-json"

const nameLimit = 100

function retryTwice(count: number, error: unknown): boolean {
  return count < 2 && isRetryable(error)
}

export type NameScope = {
  [K in keyof NameListParams]?: NameListParams[K] | undefined
}

function nameParams({
  search,
  metric,
  table,
  from,
  to,
}: NameScope): NameListParams {
  return {
    ...(search === undefined ? {} : { search }),
    ...(metric === undefined ? {} : { metric }),
    ...(table === undefined ? {} : { table }),
    ...(from === undefined ? {} : { from }),
    ...(to === undefined ? {} : { to }),
  }
}

function nameQuery({ search, ...params }: NameListParams) {
  return {
    ...(search ? { search } : {}),
    ...params,
    limit: nameLimit,
  }
}

export function metricNamesQuery(uid: string, search: string) {
  return queryOptions({
    queryKey: datasourceKeys.metrics(uid, search),
    queryFn: ({ signal }) =>
      panelRequestLimiter.run(
        () =>
          api
            .GET("/datasources/{uid}/metrics", {
              params: {
                path: { uid },
                query: search
                  ? { search, limit: nameLimit }
                  : { limit: nameLimit },
              },
              signal,
            })
            .then(unwrap),
        signal
      ),
    staleTime: 60_000,
    retry: retryTwice,
  })
}

export function labelNamesQuery(uid: string, scope: NameScope) {
  const params = nameParams(scope)
  return queryOptions({
    queryKey: datasourceKeys.labels(uid, params),
    queryFn: ({ signal }) =>
      panelRequestLimiter.run(
        () =>
          api
            .GET("/datasources/{uid}/labels", {
              params: { path: { uid }, query: nameQuery(params) },
              signal,
            })
            .then(unwrap),
        signal
      ),
    staleTime: 60_000,
    retry: retryTwice,
  })
}

export function labelValuesQuery(uid: string, label: string, scope: NameScope) {
  const params = nameParams(scope)
  return queryOptions({
    queryKey: datasourceKeys.labelValues(uid, label, params),
    queryFn: ({ signal }) =>
      panelRequestLimiter.run(
        () =>
          api
            .GET("/datasources/{uid}/labels/{name}/values", {
              params: {
                path: { uid, name: label },
                query: nameQuery(params),
              },
              signal,
            })
            .then(unwrap),
        signal
      ),
    staleTime: 60_000,
    retry: retryTwice,
  })
}

export type PreviewArgs = {
  dashboardId: string
  version: number
  panel: DataPanel
  from?: string | undefined
  to?: string | undefined
  timezone?: string | undefined
  selections: Selections
  maxDataPoints?: number | undefined
}

function previewKey(
  panel: EditablePanel,
  {
    from,
    to,
    selections,
    maxDataPoints,
  }: Pick<PreviewArgs, "from" | "to" | "selections" | "maxDataPoints">
): PreviewKeyParams {
  return {
    panel: stableJson(panel),
    ...(from === undefined ? {} : { from }),
    ...(to === undefined ? {} : { to }),
    selections,
    ...(maxDataPoints === undefined ? {} : { maxDataPoints }),
  }
}

export function previewQuery({
  dashboardId,
  version,
  panel,
  from,
  to,
  timezone,
  selections,
  maxDataPoints,
}: PreviewArgs) {
  const tz = requestZone(timezone)
  const datasource = panel.datasource ?? ""
  return queryOptions<PanelDataResponse>({
    queryKey: editorKeys.preview(
      dashboardId,
      version,
      panel.id,
      previewKey(panel, {
        from,
        to,
        selections,
        maxDataPoints,
      })
    ),
    queryFn: ({ signal }) =>
      panelRequestLimiter.run(
        () =>
          api
            .POST("/datasources/{uid}/preview", {
              params: {
                path: {
                  uid:
                    resolveDatasourceRef(datasource, selections) ?? datasource,
                },
              },
              body: {
                dashboard_id: dashboardId,
                version,
                panel_id: panel.id,
                panel: { ...panel },
                ...(from !== undefined && { from }),
                ...(to !== undefined && { to }),
                ...(tz !== undefined && { tz }),
                ...(maxDataPoints !== undefined && {
                  max_data_points: maxDataPoints,
                }),
                variables: selections,
              },
              signal,
            })
            .then(unwrap),
        signal
      ),
    staleTime: 30_000,
    gcTime: 60_000,
    retry: retryTwice,
    placeholderData: keepPreviousData,
  })
}

export type BlockPreviewArgs = Omit<PreviewArgs, "dashboardId"> & {
  observationId: string
  blockId: string | null
}

export function storedBlockPanel(
  panel: EditablePanel
): Record<string, unknown> {
  const copy: Record<string, unknown> = { ...panel }
  delete copy.id
  delete copy.gridPos
  return copy
}

export function blockPreviewQuery({
  observationId,
  blockId,
  version,
  panel,
  from,
  to,
  timezone,
  selections,
  maxDataPoints,
}: BlockPreviewArgs) {
  const tz = requestZone(timezone)
  return queryOptions<PanelDataResponse>({
    queryKey: editorKeys.blockPreview(
      observationId,
      blockId,
      version,
      previewKey(panel, {
        from,
        to,
        selections,
        maxDataPoints,
      })
    ),
    queryFn: ({ signal }) =>
      panelRequestLimiter.run(
        () =>
          api
            .POST("/observations/{id}/blocks/preview", {
              params: { path: { id: observationId } },
              body: {
                ...(blockId !== null && { block_id: blockId }),
                panel: storedBlockPanel(panel),
                ...(from !== undefined && { from }),
                ...(to !== undefined && { to }),
                ...(tz !== undefined && { tz }),
                ...(maxDataPoints !== undefined && {
                  max_data_points: maxDataPoints,
                }),
                variables: selections,
              },
              signal,
            })
            .then(unwrap),
        signal
      ),
    staleTime: 30_000,
    gcTime: 60_000,
    retry: retryTwice,
    placeholderData: keepPreviousData,
  })
}

export type CreateSuggestionVariables = {
  version: number
  panel_id: string
  prompt: string
  panel?: EditablePanel
}

export function useCreateSuggestion(dashboardId: string) {
  return useMutation({
    mutationFn: ({ panel, ...rest }: CreateSuggestionVariables) =>
      api
        .POST("/dashboards/{id}/suggestions", {
          params: { path: { id: dashboardId } },
          body: { ...rest, ...(panel ? { panel: { ...panel } } : {}) },
        })
        .then(unwrap)
        .then((created) => created.suggestion),
  })
}

export function suggestionQuery(dashboardId: string, suggestionId: string) {
  return queryOptions({
    queryKey: editorKeys.suggestion(dashboardId, suggestionId),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards/{id}/suggestions/{suggestion_id}", {
          params: { path: { id: dashboardId, suggestion_id: suggestionId } },
          signal,
        })
        .then(unwrap)
        .then((found) => found.suggestion),
    refetchInterval: (query) => {
      const status = query.state.data?.status
      if (status === "succeeded" || status === "failed") return false
      if (query.state.status === "error" && !isRetryable(query.state.error)) {
        return false
      }
      return 1000
    },
    staleTime: 0,
    retry: (count, error) => count < 3 && isRetryable(error),
  })
}

export function describeSuggestionError(error: unknown): string {
  if (!(error instanceof ApiError)) return "The request failed. Try again."
  if (error.status === 429 && error.code === "too_many_suggestions") {
    return "You already have 2 suggestions running. Wait for one to finish."
  }
  if (error.status === 429 && error.code === "agent_busy") {
    return "The agent is busy. Try again in a minute."
  }
  if (error.status === 503 && error.code === "agent_unavailable") {
    return "No model is connected to this server."
  }
  if (error.status === 422 && error.code === "invalid_spec") {
    return "Your draft is not valid yet. Fix it before asking the agent."
  }
  if (error.status === 422 && error.code === "unknown_panel") {
    return missingPanelMessage
  }
  return sentence(error.message)
}
