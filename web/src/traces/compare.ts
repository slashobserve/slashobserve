import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { isRetryable, unwrap } from "@/api/errors"
import { exploreKeys, type TraceCompareParams } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { panelRequestLimiter } from "@/dashboards/limiter"

import { inSelection, type Selection } from "./scatter"
import type { TraceRow } from "./trace-rows"

export type TraceCompare = components["schemas"]["TraceCompare"]
export type TraceDifference = components["schemas"]["TraceDifference"]

const maxSelected = 50
const maxBaseline = 150
const strongGapBasisPoints = 2000

function newestFirst(rows: TraceRow[]): TraceRow[] {
  return [...rows].sort(
    (a, b) => (b.start ?? -Infinity) - (a.start ?? -Infinity)
  )
}

export function compareBody(
  rows: readonly TraceRow[],
  selection: Selection,
  range: { from: string; to: string }
): TraceCompareParams {
  const selected: TraceRow[] = []
  const others: TraceRow[] = []
  for (const row of rows) {
    ;(inSelection(row, selection) ? selected : others).push(row)
  }
  return {
    selected: newestFirst(selected)
      .slice(0, maxSelected)
      .map((row) => row.traceId),
    baseline: newestFirst(others)
      .slice(0, maxBaseline)
      .map((row) => row.traceId),
    from: range.from,
    to: range.to,
  }
}

function sortedIds(ids: string[]): string[] {
  return [...ids].sort()
}

function traceCompareRequest(
  uid: string,
  body: TraceCompareParams,
  signal?: AbortSignal
): Promise<TraceCompare> {
  return panelRequestLimiter.run(
    () =>
      api
        .POST("/datasources/{uid}/trace-compare", {
          params: { path: { uid } },
          body,
          signal,
        })
        .then(unwrap),
    signal
  )
}

export function traceCompareQuery(uid: string, body: TraceCompareParams) {
  return queryOptions({
    queryKey: exploreKeys.traceCompare(uid, {
      ...body,
      selected: sortedIds(body.selected),
      baseline: sortedIds(body.baseline),
    }),
    queryFn: ({ signal }) => traceCompareRequest(uid, body, signal),
    staleTime: 30_000,
    retry: (count, error) => count < 1 && isRetryable(error),
    refetchOnWindowFocus: false,
  })
}

function gapBasisPoints(difference: TraceDifference): number {
  return Math.round(
    Math.abs(difference.selected_share - difference.baseline_share) * 10_000
  )
}

export function compareRows(result: TraceCompare): {
  strong: TraceDifference[]
  weak: TraceDifference[]
} {
  return {
    strong: result.differences.filter(
      (d) => gapBasisPoints(d) >= strongGapBasisPoints
    ),
    weak: result.differences.filter(
      (d) => gapBasisPoints(d) < strongGapBasisPoints
    ),
  }
}

function percent(share: number): number {
  return Math.round(share * 100)
}

export function percentText(share: number): string {
  return `${percent(share)}%`
}

export function shareText(difference: TraceDifference): string {
  return `${percent(difference.selected_share)} percent of selected traces, ${percent(difference.baseline_share)} percent of the rest`
}

export function attributeKey(attribute: string): string {
  return attribute.replace(/^(span|resource)\./, "")
}

function count(n: number): string {
  return n.toLocaleString("en")
}

export function compareDescription(result: TraceCompare): string {
  return `Attribute values that appear far more often in the ${count(result.selected.read)} selected traces than in the other ${count(result.baseline.read)}. Ranked by the size of the gap.`
}

export function compareNote(
  result: TraceCompare,
  selectedCount: number
): string {
  const parts = [
    `Compared ${count(result.attributes)} attributes across the spans of each trace. Attributes with one value everywhere are left out.`,
  ]
  if (result.selected.requested < selectedCount) {
    parts.push(
      `Compared the newest ${count(result.selected.requested)} of the ${count(selectedCount)} selected traces.`
    )
  }
  if (result.partial) {
    const unread =
      result.selected.requested -
      result.selected.read +
      result.baseline.requested -
      result.baseline.read
    parts.push(
      `${count(unread)} traces were not read before the time limit; the shares are from the rest.`
    )
  }
  return parts.join(" ")
}
