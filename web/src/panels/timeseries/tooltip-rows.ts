export type TooltipSort = "none" | "asc" | "desc"

export type TooltipRow = { index: number; value: number }

export type StackEntry = {
  stacked: boolean
  below: number | null
  value: number
}

export type TooltipRowOptions = {
  sort: TooltipSort
  hideZeros: boolean
  cumulative: boolean
  stack: readonly StackEntry[]
}

export type TooltipRows = { rows: TooltipRow[]; total: number }

function stackedTotals(stack: readonly StackEntry[]): number[] {
  const totals: number[] = []
  stack.forEach((entry, index) => {
    const lower = entry.below === null ? 0 : totals[entry.below]
    totals[index] = entry.stacked ? entry.value + lower : entry.value
  })
  return totals
}

export function tooltipRows(
  rows: readonly TooltipRow[],
  { sort, hideZeros, cumulative, stack }: TooltipRowOptions,
  limit: number
): TooltipRows {
  const totals = cumulative ? stackedTotals(stack) : []
  const shown = rows
    .filter((row) => !hideZeros || row.value !== 0)
    .map((row) =>
      cumulative && stack[row.index]?.stacked
        ? { index: row.index, value: totals[row.index] }
        : row
    )
  if (sort === "asc") shown.sort((a, b) => a.value - b.value)
  if (sort === "desc") shown.sort((a, b) => b.value - a.value)
  return { rows: shown.slice(0, limit), total: shown.length }
}
