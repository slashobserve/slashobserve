import {
  clip,
  inRect,
  msToIso,
  positionOf,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "../anchors"
import type { LogEntry } from "./logs-model"

export type LogAnchorLayout = {
  body: Size
  lines: readonly {
    rect: Rect
    entry: Pick<LogEntry, "key" | "time" | "message" | "labels">
  }[]
}

export function hitTest(layout: LogAnchorLayout, point: Point): Anchor | null {
  const line = layout.lines.find(({ rect }) => inRect(rect, point))
  if (line === undefined) return null
  const { entry } = line
  const labels = Object.entries(entry.labels).map(
    ([key, value]) => [key, clip(value)] as const
  )
  return {
    kind: "log_line",
    pos: positionOf(point, layout.body),
    id: clip(entry.key),
    ...(entry.time === null ? {} : { from: msToIso(entry.time) }),
    ...(entry.message ? { quote: clip(entry.message) } : {}),
    ...(labels.length > 0 ? { series: Object.fromEntries(labels) } : {}),
  }
}
