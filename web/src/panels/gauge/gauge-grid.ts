export const gridGap = 8

export type GaugeGrid = { columns: number; rows: number }

function cellSide(
  columns: number,
  rows: number,
  width: number,
  height: number
): number {
  return Math.min(
    (width - gridGap * (columns - 1)) / columns,
    (height - gridGap * (rows - 1)) / rows
  )
}

export function gaugeGrid(
  count: number,
  width: number,
  height: number
): GaugeGrid {
  const total = Math.max(count, 1)
  let best: GaugeGrid = { columns: 1, rows: total }
  let bestSide = cellSide(1, total, width, height)
  for (let columns = 2; columns <= total; columns++) {
    const rows = Math.ceil(total / columns)
    const side = cellSide(columns, rows, width, height)
    if (side > bestSide) {
      best = { columns, rows }
      bestSide = side
    }
  }
  return best
}
