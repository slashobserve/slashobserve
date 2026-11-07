import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { isRetryable, unwrap } from "@/api/errors"
import { exploreKeys } from "@/api/keys"
import { panelRequestLimiter } from "@/dashboards/limiter"
import type { PanelDataResponse } from "@/dashboards/queries"
import type { TimeRangeValue } from "@/dashboards/time-range-picker"
import { requestZone } from "@/lib/time"
import { formatValue } from "@/lib/units"

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

type TraceMeta = PanelDataResponse["meta"]

function isCut(meta: TraceMeta): meta is TraceMeta & { rows_total: number } {
  return meta.truncated && meta.rows_total !== undefined
}

export function traceNotes(meta: TraceMeta): string[] {
  const notes: string[] = []
  if (meta.partial) {
    notes.push("Tempo did not search every block; some spans may be missing.")
  }
  return notes
}

export function traceSizeText(meta: TraceMeta): string | null {
  return meta.bytes === undefined || meta.bytes <= 0
    ? null
    : `about ${formatValue(meta.bytes, "decbytes", 0)}`
}

export function criticalPathDisabledText(meta: TraceMeta): string | undefined {
  return isCut(meta)
    ? `Critical path needs all spans. This trace is cut at ${meta.rows.toLocaleString("en")}.`
    : undefined
}

export function traceNotice(
  meta: TraceMeta,
  spanCount: number,
  folded: boolean
): string | null {
  const size = traceSizeText(meta)
  const sized = (text: string) => (size === null ? text : `${text} (${size})`)
  if (isCut(meta)) {
    return `This trace has ${meta.rows_total.toLocaleString("en")} spans. The first ${sized(meta.rows.toLocaleString("en"))} by start time are loaded; the rest are not.`
  }
  if (!folded) return null
  return `This trace has ${sized(`${spanCount.toLocaleString("en")} spans`)}. Repeated spans are folded into their parent so the page stays quick. Nothing is lost: every folded group can be opened.`
}
