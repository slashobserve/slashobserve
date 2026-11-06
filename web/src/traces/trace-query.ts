import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { isRetryable, unwrap } from "@/api/errors"
import { exploreKeys } from "@/api/keys"
import { panelRequestLimiter } from "@/dashboards/limiter"
import type { PanelDataResponse } from "@/dashboards/queries"
import type { TimeRangeValue } from "@/dashboards/time-range-picker"
import { requestZone } from "@/lib/time"

function traceRequest(
  uid: string,
  traceId: string,
  range: TimeRangeValue,
  signal?: AbortSignal
): Promise<PanelDataResponse> {
  const timezone = requestZone("browser")
  return panelRequestLimiter.run(
    () =>
      api
        .GET("/datasources/{uid}/traces/{trace_id}", {
          params: {
            path: { uid, trace_id: traceId },
            query: {
              from: range.from,
              to: range.to,
              ...(timezone !== undefined && { timezone }),
            },
          },
          signal,
        })
        .then(unwrap),
    signal
  )
}

export function traceQuery(
  uid: string,
  traceId: string,
  range: TimeRangeValue
) {
  return queryOptions({
    queryKey: exploreKeys.trace(uid, traceId, range),
    queryFn: ({ signal }) => traceRequest(uid, traceId, range, signal),
    staleTime: 60_000,
    retry: (count, error) => count < 1 && isRetryable(error),
    refetchOnWindowFocus: false,
  })
}

export function traceNotes(meta: PanelDataResponse["meta"]): string[] {
  const notes: string[] = []
  if (meta.truncated && meta.rows_total !== undefined) {
    notes.push(
      `Showing the first ${meta.rows.toLocaleString("en")} of ${meta.rows_total.toLocaleString("en")} spans by start time. The rest are not loaded.`
    )
  }
  if (meta.partial) {
    notes.push("Tempo did not search every block; some spans may be missing.")
  }
  return notes
}
