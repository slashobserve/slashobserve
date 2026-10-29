import type { WireFrame } from "@/dashboards/queries"

export type WireField = WireFrame["fields"][number]
export type ColumnType = WireField["type"]

const legendToken = /\{\{\s*([^{}\s]+)\s*\}\}/g

function escapeLabelValue(value: string): string {
  return value.replaceAll("\\", "\\\\").replaceAll('"', '\\"')
}

export function seriesLabel(
  frame: WireFrame,
  field: WireField,
  legend: string | undefined,
  numberFieldCount: number
): string {
  let label: string
  if (legend) {
    label = legend.replace(legendToken, (_match, key: string) =>
      key === "__name__" ? (frame.name ?? "") : (frame.labels?.[key] ?? "")
    )
  } else {
    const keys = Object.keys(frame.labels ?? {}).sort()
    if (keys.length > 0) {
      const pairs = keys.map(
        (key) => `${key}="${escapeLabelValue(frame.labels?.[key] ?? "")}"`
      )
      label = `${frame.name ?? ""}{${pairs.join(", ")}}`
    } else if (frame.name !== undefined) {
      label = frame.name
    } else {
      return field.name
    }
  }
  return numberFieldCount > 1 ? `${label} ${field.name}` : label
}

export function qualifyCollidingLabels<
  T extends { ref: string; label: string },
>(series: T[]): T[] {
  const refsByLabel = new Map<string, Set<string>>()
  for (const { ref, label } of series) {
    const refs = refsByLabel.get(label) ?? new Set<string>()
    refs.add(ref)
    refsByLabel.set(label, refs)
  }
  return series.map((item) =>
    (refsByLabel.get(item.label)?.size ?? 0) > 1
      ? { ...item, label: `${item.ref}: ${item.label}` }
      : item
  )
}

export function seriesKey(frame: WireFrame, fieldName: string): string {
  const labels = Object.entries(frame.labels ?? {})
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([key, value]) => `${key}=${value}`)
    .join(",")
  return `${frame.ref}:${frame.name ?? ""}{${labels}}#${fieldName}`
}

export function uniqueKey(seen: Map<string, number>, base: string): string {
  const count = seen.get(base) ?? 0
  if (count === 0) {
    seen.set(base, 1)
    return base
  }
  let next = count
  while (seen.has(`${base}#${next}`)) next++
  const name = `${base}#${next}`
  seen.set(base, next + 1)
  seen.set(name, 1)
  return name
}

export function hasData(frames: WireFrame[]): boolean {
  return frames.some((frame) =>
    frame.fields.some(
      (field) =>
        field.type !== "time" &&
        field.values.some((value) => value !== null && value !== undefined)
    )
  )
}
