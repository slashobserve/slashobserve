import {
  clip,
  inRect,
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

const maxRowLabels = 16

export function hitTest(
  layout: AlertAnchorLayout,
  point: Point
): Anchor | null {
  const found = layout.rows.find(({ rect }) => inRect(rect, point))
  if (found === undefined) return null
  const { alert } = found
  const labels = [["alertname", alert.name], ...alert.labels]
    .filter(([, value]) => value !== "")
    .slice(0, maxRowLabels)
  return {
    kind: "row",
    pos: positionOf(point, layout.body),
    row: Object.fromEntries(
      labels.map(([key, value]) => [clip(key), clip(value)])
    ),
  }
}
