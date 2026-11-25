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
