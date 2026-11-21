import {
  clip,
  fit,
  inRect,
  labelsOf,
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
  const labels = labelsOf(Object.entries(entry.labels))
  const from = entry.time === null ? null : msToIso(entry.time)
  return fit({
    kind: "log_line",
    pos: positionOf(point, layout.body),
    id: clip(entry.key),
    ...(from === null ? {} : { from }),
    ...(entry.message ? { quote: clip(entry.message) } : {}),
    ...(Object.keys(labels).length > 0 ? { series: labels } : {}),
  })
}
