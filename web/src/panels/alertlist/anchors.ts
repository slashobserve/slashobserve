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
import { bottomEdge, revealKey } from "../list-anchors"
import type { Alert } from "./alert-model"

type AnchorAlert = Pick<Alert, "name" | "labels">
type HiddenAlert = AnchorAlert & Pick<Alert, "key">

export type AlertAnchorLayout = {
  body: Size
  rows: readonly { rect: Rect; alert: AnchorAlert }[]
  elsewhere?: readonly HiddenAlert[]
  reveal?: (key: string) => void
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

function hiddenAlert(
  layout: AlertAnchorLayout,
  anchor: Anchor
): HiddenAlert | undefined {
  if (anchor.kind !== "row") return undefined
  if (layout.rows.some(({ alert }) => named(alert, anchor))) return undefined
  return (layout.elsewhere ?? []).find((alert) => named(alert, anchor))
}

function named(alert: AnchorAlert, anchor: Anchor): boolean {
  return namesLabels(anchor.row, alertLabels(alert))
}

export function locate(layout: AlertAnchorLayout, anchor: Anchor): Rect | null {
  if (anchor.kind !== "row") return null
  const drawn = layout.rows.find(({ alert }) => named(alert, anchor))
  if (drawn !== undefined) return drawn.rect
  return hiddenAlert(layout, anchor) === undefined
    ? null
    : bottomEdge(layout.body)
}

export function reveal(layout: AlertAnchorLayout, anchor: Anchor): boolean {
  return revealKey(layout.reveal, hiddenAlert(layout, anchor)?.key)
}
