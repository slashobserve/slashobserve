import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { isRetryable, unwrap } from "@/api/errors"
import type { components } from "@/api/schema.gen"
import { panelRequestLimiter } from "@/dashboards/limiter"
import type { LogLevelKind } from "@/panels/logs/logs-model"
import { describeRange, zonedDateTime } from "@/lib/time"
import { lowerFirst } from "@/lib/text"

export type LogVolume = components["schemas"]["LogVolume"]
export type LogVolumeBucket = components["schemas"]["LogVolumeBucket"]
export type VolumeParams = { query: string; from: string; to: string }
export type VolumeTotals = Record<LogLevelKind, number>

export const volumeKinds: readonly LogLevelKind[] = [
  "error",
  "warning",
  "info",
  "other",
]

export const volumeKindNames: Record<LogLevelKind, string> = {
  error: "Error",
  warning: "Warning",
  info: "Info",
  other: "Other",
}

const volumeBuckets = 60

const kindNouns: Record<LogLevelKind, readonly [string, string]> = {
  error: ["error", "errors"],
  warning: ["warning", "warnings"],
  info: ["info", "info"],
  other: ["other", "other"],
}

function logVolumeRequest(
  uid: string,
  { query, from, to }: VolumeParams,
  signal?: AbortSignal
): Promise<LogVolume> {
  return panelRequestLimiter.run(
    () =>
      api
        .POST("/datasources/{uid}/log-volume", {
          params: { path: { uid } },
          body: { query, from, to, buckets: volumeBuckets },
          signal,
        })
        .then(unwrap),
    signal
  )
}

export function logVolumeQuery(uid: string, params: VolumeParams) {
  return queryOptions({
    queryKey: ["explore", uid, "volume", params],
    queryFn: ({ signal }) => logVolumeRequest(uid, params, signal),
    staleTime: 0,
    gcTime: 60_000,
    retry: (count, error) => count < 1 && isRetryable(error),
    refetchOnWindowFocus: false,
  })
}

export function volumeTotals(
  buckets: readonly LogVolumeBucket[]
): VolumeTotals {
  const totals: VolumeTotals = { error: 0, warning: 0, info: 0, other: 0 }
  for (const bucket of buckets) {
    for (const kind of volumeKinds) totals[kind] += bucket[kind]
  }
  return totals
}

function count(value: number): string {
  return value.toLocaleString("en")
}

export function bucketTotal(
  bucket: LogVolumeBucket,
  kinds: readonly LogLevelKind[] = volumeKinds
): number {
  return kinds.reduce((sum, kind) => sum + bucket[kind], 0)
}

function lineCount(lines: number): string {
  return `${count(lines)} ${lines === 1 ? "line" : "lines"}`
}

export function volumeBucketText(bucket: LogVolumeBucket): string {
  const parts = volumeKinds
    .filter((kind) => bucket[kind] > 0)
    .map((kind) => {
      const [one, many] = kindNouns[kind]
      return `${count(bucket[kind])} ${bucket[kind] === 1 ? one : many}`
    })
  const lines = lineCount(bucketTotal(bucket))
  return parts.length === 0 ? lines : `${lines}: ${parts.join(", ")}`
}

export function selectionEdges(
  volume: Pick<LogVolume, "buckets" | "step_ms">,
  fromIndex: number,
  toIndex: number,
  zone: string | undefined
): { start: string; end: string } {
  const startMs = Date.parse(volume.buckets[fromIndex].start)
  const endMs = Date.parse(volume.buckets[toIndex].start) + volume.step_ms
  const start = zonedDateTime(startMs, zone)
  const end = zonedDateTime(endMs, zone)
  const cut = volume.step_ms % 60_000 === 0 ? 16 : 19
  const sameDay = start.slice(0, 10) === end.slice(0, 10)
  return sameDay
    ? { start: start.slice(11, cut), end: end.slice(11, cut) }
    : { start: start.slice(0, cut), end: end.slice(0, cut) }
}

export function selectionText(
  volume: Pick<LogVolume, "buckets" | "step_ms">,
  fromIndex: number,
  toIndex: number,
  zone: string | undefined
): string {
  const { start, end } = selectionEdges(volume, fromIndex, toIndex, zone)
  const lines = bucketTotal({
    start: "",
    ...volumeTotals(volume.buckets.slice(fromIndex, toIndex + 1)),
  })
  return `Selected ${start} to ${end}, ${lineCount(lines)}`
}

export function volumeFailureText(reason: string): string {
  return `The volume query did not finish (${reason.replace(/\.$/, "")}). The lines below are complete.`
}

export function volumeLabel(total: number, totals: VolumeTotals): string {
  const lines = `${count(total)} lines`
  const parts = volumeKinds
    .filter((kind) => totals[kind] > 0)
    .map((kind) => `${count(totals[kind])} ${kind}`)
  return parts.length === 0 ? lines : `${lines}: ${parts.join(", ")}`
}

export function volumeCaption(total: number, from: string, to: string) {
  const range = describeRange(from, to)
  const words = range.startsWith("Last ")
    ? `in the ${lowerFirst(range)}`
    : "in this range"
  return `${count(total)} lines ${words}`
}
