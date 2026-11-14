import type { TraceSearch } from "./search"

export const traceFilterLabels = {
  service: "Service",
  span: "Span name",
  min: "Min duration",
  max: "Max duration",
  errors: "Errors only",
  q: "TraceQL",
} as const

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

export function traceFilterChips(search: TraceSearch): TraceFilterChip[] {
  return chipOrder.flatMap((key) => {
    const value = search[key]
    if (value === undefined) return []
    return [
      {
        key,
        label: traceFilterLabels[key],
        value: key === "errors" ? "" : value,
      },
    ]
  })
}
