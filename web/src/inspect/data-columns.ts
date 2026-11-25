const charPx = 7.5
const cellPadPx = 28
const minPx = 64
const maxPx = 320
const sampledRows = 50

export function dataColumnWidth(
  header: string,
  rows: readonly (readonly string[])[],
  column: number
): number {
  const longest = rows
    .slice(0, sampledRows)
    .reduce(
      (most, row) => Math.max(most, (row[column] ?? "").length),
      header.length
    )
  return Math.min(
    maxPx,
    Math.max(minPx, Math.ceil(longest * charPx) + cellPadPx)
  )
}

const squeezedPx = 96

export function fitColumnWidths(
  natural: readonly number[],
  available: number
): number[] {
  const total = natural.reduce((sum, width) => sum + width, 0)
  if (available <= 0 || total <= available) return [...natural]
  const floors = natural.map((width) => Math.min(width, squeezedPx))
  const floorTotal = floors.reduce((sum, width) => sum + width, 0)
  if (floorTotal >= available) return [...natural]
  const share = (available - floorTotal) / (total - floorTotal)
  return natural.map((width, index) =>
    Math.floor(floors[index] + (width - floors[index]) * share)
  )
}
