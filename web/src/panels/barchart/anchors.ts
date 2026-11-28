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
  const entry = model.categories[category]
  if (entry === undefined) return null
  const label = clip(entry.label)
  if (bar === null) return fit({ kind: "bar", pos, category: label })
  const series = model.series[bar]
  const cell = model.bars[category]?.[bar]
  if (series === undefined || cell === undefined) return null
  const shown = clip(cell.shown.text)
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
  const series = model.series[index]
  if (bar === undefined || bar.value === null || series === undefined) {
    return null
  }
  return shifted(barRect(layout, category, series.slot, bar), origin)
}
