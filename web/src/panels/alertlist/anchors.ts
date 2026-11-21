import {
  fit,
  inRect,
  labelsOf,
  namesLabels,
  positionOf,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "../anchors"
import { bottomEdge } from "../list-anchors"
import type { Alert } from "./alert-model"

type AnchorAlert = Pick<Alert, "name" | "labels">

export type AlertAnchorLayout = {
  body: Size
  rows: readonly { rect: Rect; alert: AnchorAlert }[]
  elsewhere?: readonly AnchorAlert[]
}

function alertLabels(alert: AnchorAlert): Record<string, string> {
  return labelsOf(
    [["alertname", alert.name] as const, ...alert.labels].filter(
      ([, value]) => value !== ""
    )
  )
}

export function hitTest(
  layout: AlertAnchorLayout,
  point: Point
): Anchor | null {
  const found = layout.rows.find(({ rect }) => inRect(rect, point))
  if (found === undefined) return null
  return fit({
    kind: "row",
    pos: positionOf(point, layout.body),
    row: alertLabels(found.alert),
  })
}

export function locate(layout: AlertAnchorLayout, anchor: Anchor): Rect | null {
  if (anchor.kind !== "row") return null
  const named = (alert: AnchorAlert) =>
    namesLabels(anchor.row, alertLabels(alert))
  const drawn = layout.rows.find(({ alert }) => named(alert))
  if (drawn !== undefined) return drawn.rect
  return (layout.elsewhere ?? []).some(named) ? bottomEdge(layout.body) : null
}
