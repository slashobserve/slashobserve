import {
  clip,
  fit,
  positionOf,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "../anchors"
import {
  barRect,
  categoryBand,
  hitTest as barAt,
  type BarchartModel,
  type Layout,
} from "./barchart-model"

export type BarAnchorLayout = {
  body: Size
  origin: Point
  layout: Layout
  model: Pick<BarchartModel, "categories" | "series" | "bars">
}

export function hitTest(
  { body, origin, layout, model }: BarAnchorLayout,
  point: Point
): Anchor | null {
  const x = point.x - origin.x
  const y = point.y - origin.y
  if (x < 0 || y < 0 || x > layout.plot.width || y > layout.plot.height) {
    return null
  }
  if (
    !(layout.count > 0) ||
    !(layout.plot.width > 0 && layout.plot.height > 0)
  ) {
    return null
  }
  const { category, bar } = barAt(layout, model, x, y)
  const pos = positionOf(point, body)
  const label = clip(model.categories[category].label)
  if (bar === null) return fit({ kind: "bar", pos, category: label })
  const series = model.series[bar]
  const shown = clip(model.bars[category][bar].shown.text)
  return fit({
    kind: series.stack === "none" ? "bar" : "segment",
    pos,
    category: label,
    series: { name: clip(series.name) },
    shown,
  })
}

function shifted(rect: Rect, origin: Point): Rect {
  return { ...rect, x: rect.x + origin.x, y: rect.y + origin.y }
}

export function locate(
  { origin, layout, model }: BarAnchorLayout,
  anchor: Anchor
): Rect | null {
  if (anchor.kind !== "bar" && anchor.kind !== "segment") return null
  const category = model.categories.findIndex(
    ({ label }) => clip(label) === anchor.category
  )
  if (category < 0 || category >= layout.count) return null
  const name = anchor.series?.name
  if (name === undefined) return shifted(categoryBand(layout, category), origin)
  const index = model.series.findIndex((series) => clip(series.name) === name)
  const bar = index < 0 ? undefined : model.bars[category]?.[index]
  if (bar === undefined || bar.value === null) return null
  return shifted(
    barRect(layout, category, model.series[index].slot, bar),
    origin
  )
}
