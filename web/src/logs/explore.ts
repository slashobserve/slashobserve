import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import {
  ApiError,
  failedQueryText,
  isRetryable,
  NetworkError,
  unwrap,
} from "@/api/errors"
import { exploreKeys, type ExploreParams } from "@/api/keys"
import { panelRequestLimiter } from "@/dashboards/limiter"
import type { PanelDataResponse } from "@/dashboards/queries"
import { requestZone } from "@/lib/time"

export const offlineText =
  "You are offline. The query runs when the connection is back."

export function exploreRequest(
  uid: string,
  { query, kind, from, to, limit, maxDataPoints }: ExploreParams,
  signal?: AbortSignal
): Promise<PanelDataResponse> {
  const timezone = requestZone("browser")
  return panelRequestLimiter.run(
    () =>
      api
        .POST("/datasources/{uid}/query", {
          params: { path: { uid } },
          body: {
            query,
            kind,
            from,
            to,
            ...(timezone !== undefined && { timezone }),
            ...(limit !== undefined && { limit }),
            ...(maxDataPoints !== undefined && {
              max_data_points: maxDataPoints,
            }),
          },
          signal,
        })
        .then(unwrap),
    signal
  )
}

export function exploreQuery(uid: string, params: ExploreParams) {
  return queryOptions({
    queryKey: exploreKeys.query(uid, params),
    queryFn: ({ signal }) => exploreRequest(uid, params, signal),
    staleTime: 0,
    gcTime: 60_000,
    retry: (count, error) => count < 1 && isRetryable(error),
    placeholderData: keepPreviousData,
    refetchOnWindowFocus: false,
  })
}

export function exploreErrorText(error: unknown): string {
  if (error instanceof ApiError) return failedQueryText(error)
  if (error instanceof NetworkError) return offlineText
  return "The query failed. Try again."
}
