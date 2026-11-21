import {
  clip,
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
  elsewhere?: readonly (readonly string[])[]
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

function rowRect(layout: TableAnchorLayout, row: AnchorRow): Rect {
  return { x: 0, y: row.top, width: layout.body.width, height: row.height }
}

function columnRect(
  layout: TableAnchorLayout,
  column: AnchorColumn,
  row: AnchorRow | null
): Rect {
  const header = layout.header ?? { y: 0, height: 0 }
  return row === null
    ? {
        x: column.left,
        y: header.y,
        width: column.width,
        height: header.height,
      }
    : { x: column.left, y: row.top, width: column.width, height: row.height }
}

function drawnRow(layout: TableAnchorLayout, anchor: Anchor): AnchorRow | null {
  return (
    layout.rows.find((row) =>
      namesLabels(anchor.row, rowLabels(layout, row))
    ) ?? null
  )
}

function rowElsewhere(layout: TableAnchorLayout, anchor: Anchor): boolean {
  return (layout.elsewhere ?? []).some((cells) =>
    namesLabels(anchor.row, rowLabels(layout, { top: 0, height: 0, cells }))
  )
}

export function locate(layout: TableAnchorLayout, anchor: Anchor): Rect | null {
  const column = layout.columns.find(({ name }) => clip(name) === anchor.column)
  switch (anchor.kind) {
    case "column":
      return column === undefined ? null : columnRect(layout, column, null)
    case "row":
    case "cell": {
      if (anchor.kind === "cell" && column === undefined) return null
      const row = drawnRow(layout, anchor)
      if (row === null)
        return rowElsewhere(layout, anchor) ? bottomEdge(layout.body) : null
      return column === undefined
        ? rowRect(layout, row)
        : columnRect(layout, column, row)
    }
    default:
      return null
  }
}
