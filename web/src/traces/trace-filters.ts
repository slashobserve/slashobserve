import type { TraceSearch } from "./search"

export const traceFilterLabels = {
  service: "Service",
  span: "Span name",
  min: "Min span duration",
  max: "Max span duration",
  errors: "Errors only",
  q: "TraceQL",
} as const

export function queryLabel(type: string): string {
  return type === "jaeger" ? "Search" : traceFilterLabels.q
}

type TraceFilterKey = keyof typeof traceFilterLabels

export type TraceFilterChip = {
  key: TraceFilterKey
  label: string
  value: string
}

const chipOrder: readonly TraceFilterKey[] = [
  "service",
  "span",
  "min",
  "max",
  "errors",
  "q",
]

export function traceFilterChips(
  type: string,
  search: TraceSearch
): TraceFilterChip[] {
  return chipOrder.flatMap((key) => {
    const value = search[key]
    if (value === undefined) return []
    return [
      {
        key,
        label: key === "q" ? queryLabel(type) : traceFilterLabels[key],
        value: key === "errors" ? "" : value,
      },
    ]
  })
}
