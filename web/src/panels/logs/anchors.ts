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
import { bottomEdge, revealKey } from "../list-anchors"
import type { LogEntry } from "./logs-model"

export type LogAnchorLayout = {
  body: Size
  lines: readonly {
    rect: Rect
    entry: Pick<LogEntry, "key" | "time" | "message" | "labels">
  }[]
  resolve?: (id: string) => string | undefined
  reveal?: (key: string) => void
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

function hiddenKey(
  layout: LogAnchorLayout,
  anchor: Anchor
): string | undefined {
  if (anchor.kind !== "log_line") return undefined
  if (layout.lines.some(({ entry }) => clip(entry.key) === anchor.id)) {
    return undefined
  }
  return anchor.id === undefined ? undefined : layout.resolve?.(anchor.id)
}

export function locate(layout: LogAnchorLayout, anchor: Anchor): Rect | null {
  if (anchor.kind !== "log_line") return null
  const drawn = layout.lines.find(({ entry }) => clip(entry.key) === anchor.id)
  if (drawn !== undefined) return drawn.rect
  return hiddenKey(layout, anchor) === undefined
    ? null
    : bottomEdge(layout.body)
}

export function reveal(layout: LogAnchorLayout, anchor: Anchor): boolean {
  return revealKey(layout.reveal, hiddenKey(layout, anchor))
}
