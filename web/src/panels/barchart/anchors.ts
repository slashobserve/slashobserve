import {
  clip,
  fit,
  positionOf,
  type Anchor,
  type Point,
  type Size,
} from "../anchors"
import {
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
