import type { WireFrame } from "@/dashboards/queries"

export type ExemplarSeries = {
  key: string
  ref: string
  labels: Record<string, string> | undefined
  scale: string
}

export type ExemplarPoint = {
  x: number
  y: number
  seriesKey?: string
  scale: string
  labels: [string, string][]
}

const unmatchedScale = "y"
const traceIdKeys = ["trace_id", "traceID", "traceId"]

function sameLabels(
  a: Record<string, string> | undefined,
  b: Record<string, string> | undefined
): boolean {
  const left = Object.entries(a ?? {})
  const right = b ?? {}
  return (
    left.length === Object.keys(right).length &&
    left.every(([key, value]) => right[key] === value)
  )
}

function orderedExemplarLabels(labels: [string, string][]): [string, string][] {
  const traceId = labels.findIndex(([key]) => traceIdKeys.includes(key))
  if (traceId <= 0) return labels
  return [labels[traceId], ...labels.filter((_, index) => index !== traceId)]
}

export function exemplarPoints(
  frames: readonly WireFrame[],
  series: readonly ExemplarSeries[],
  xRange: readonly [number, number]
): ExemplarPoint[] {
  const [min, max] = xRange
  const points: ExemplarPoint[] = []
  for (const frame of frames) {
    const times = frame.fields.find((field) => field.type === "time")
    const values = frame.fields.find((field) => field.type === "number")
    if (!times || !values) continue
    const texts = frame.fields.filter((field) => field.type === "string")
    const match = series.find(
      (entry) =>
        entry.ref === frame.ref && sameLabels(entry.labels, frame.labels)
    )
    times.values.forEach((time, row) => {
      const value = values.values[row]
      if (typeof time !== "number" || typeof value !== "number") return
      if (!Number.isFinite(value)) return
      const x = time / 1000
      if (x < min || x > max) return
      const labels = texts.flatMap((field): [string, string][] => {
        const text = field.values[row]
        return typeof text === "string" && text !== ""
          ? [[field.name, text]]
          : []
      })
      points.push({
        x,
        y: value,
        ...(match ? { seriesKey: match.key } : {}),
        scale: match?.scale ?? unmatchedScale,
        labels: orderedExemplarLabels(labels),
      })
    })
  }
  return points
}

export function exemplarAt(
  points: readonly ExemplarPoint[],
  cursorX: number,
  cursorY: number,
  radiusPx: number,
  toPx: (point: ExemplarPoint) => [number, number]
): ExemplarPoint | null {
  let nearest: ExemplarPoint | null = null
  let best = Infinity
  const limit = radiusPx * radiusPx
  for (const point of points) {
    const [px, py] = toPx(point)
    const distance = (px - cursorX) ** 2 + (py - cursorY) ** 2
    if (distance <= limit && distance < best) {
      best = distance
      nearest = point
    }
  }
  return nearest
}
