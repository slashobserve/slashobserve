import {
  fit,
  inRect,
  labelsOf,
  positionOf,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "../anchors"
import type { Alert } from "./alert-model"

export type AlertAnchorLayout = {
  body: Size
  rows: readonly { rect: Rect; alert: Pick<Alert, "name" | "labels"> }[]
}

export function hitTest(
  layout: AlertAnchorLayout,
  point: Point
): Anchor | null {
  const found = layout.rows.find(({ rect }) => inRect(rect, point))
  if (found === undefined) return null
  const { alert } = found
  const labels = labelsOf(
    [["alertname", alert.name] as const, ...alert.labels].filter(
      ([, value]) => value !== ""
    )
  )
  return fit({
    kind: "row",
    pos: positionOf(point, layout.body),
    row: labels,
  })
}
