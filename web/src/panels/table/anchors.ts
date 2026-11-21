import {
  clip,
  fit,
  inRect,
  labelsOf,
  positionOf,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "../anchors"

export type AnchorColumn = {
  name: string
  left: number
  width: number
  dimension: boolean
}

export type AnchorRow = {
  top: number
  height: number
  cells: readonly string[]
}

export type TableAnchorLayout = {
  body: Size
  header: Pick<Rect, "y" | "height"> | null
  columns: readonly AnchorColumn[]
  rows: readonly AnchorRow[]
}

function columnAt(
  layout: TableAnchorLayout,
  x: number
): { index: number; column: AnchorColumn } | null {
  const index = layout.columns.findIndex(
    ({ left, width }) => x >= left && x <= left + width
  )
  return index < 0 ? null : { index, column: layout.columns[index] }
}

function rowLabels(
  layout: TableAnchorLayout,
  row: AnchorRow
): Record<string, string> {
  const named = layout.columns.flatMap((column, index) =>
    column.dimension && row.cells[index] ? [[column, row.cells[index]]] : []
  ) as [AnchorColumn, string][]
  const chosen =
    named.length > 0
      ? named
      : layout.columns.map(
          (column, index) => [column, row.cells[index] ?? ""] as const
        )
  return labelsOf(chosen.map(([column, text]) => [column.name, text]))
}

export function hitTest(
  layout: TableAnchorLayout,
  point: Point
): Anchor | null {
  const pos = positionOf(point, layout.body)
  const found = columnAt(layout, point.x)
  if (
    layout.header !== null &&
    inRect({ x: 0, width: layout.body.width, ...layout.header }, point)
  ) {
    return found === null
      ? null
      : fit({ kind: "column", pos, column: clip(found.column.name) })
  }
  const row = layout.rows.find(
    ({ top, height }) => point.y >= top && point.y <= top + height
  )
  if (row === undefined) return null
  const labels = rowLabels(layout, row)
  if (Object.keys(labels).length === 0) return null
  const under =
    found !== null && !found.column.dimension ? row.cells[found.index] : ""
  return fit({
    kind: "row",
    pos,
    row: labels,
    ...(under ? { shown: clip(under) } : {}),
  })
}
