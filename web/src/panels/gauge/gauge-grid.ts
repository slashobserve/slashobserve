export const gridGap = 8

export type GaugeGrid = { columns: number; rows: number }

export type GridLimits = { minWidth?: number; minHeight?: number }

function cell(
  columns: number,
  rows: number,
  width: number,
  height: number
): { width: number; height: number } {
  return {
    width: (width - gridGap * (columns - 1)) / columns,
    height: (height - gridGap * (rows - 1)) / rows,
  }
}

export function gaugeGrid(
  count: number,
  width: number,
  height: number,
  limits: GridLimits = {}
): GaugeGrid {
  const total = Math.max(count, 1)
  const { minWidth = 0, minHeight = 0 } = limits
  let best: GaugeGrid | null = null
  let bestSide = -Infinity
  for (let columns = 1; columns <= total; columns++) {
    const rows = Math.ceil(total / columns)
    const size = cell(columns, rows, width, height)
    if (size.width < minWidth || size.height < minHeight) continue
    const side = Math.min(size.width, size.height)
    if (side > bestSide) {
      best = { columns, rows }
      bestSide = side
    }
  }
  if (best) return best
  const fitting = Math.floor((width + gridGap) / (minWidth + gridGap))
  const columns = Math.min(total, Math.max(1, fitting))
  return { columns, rows: Math.ceil(total / columns) }
}
