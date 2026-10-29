export const tileGap = 8
const tileMinWidth = 120
const tileSqueezedWidth = 96
const tileMinHeight = 72

function tracksFor(size: number, minimum: number): number {
  return Math.max(1, Math.floor((size + tileGap) / (minimum + tileGap)))
}

export function tilesAcross(
  orientation: string,
  width: number,
  height: number,
  count: number
): number {
  if (orientation === "horizontal") return Math.max(1, count)
  if (orientation === "vertical") return 1
  const fitting = Math.min(count, tracksFor(width, tileMinWidth))
  const rowsThatFit = tracksFor(height, tileMinHeight)
  const squeezed = Math.min(
    Math.ceil(count / rowsThatFit),
    tracksFor(width, tileSqueezedWidth)
  )
  return Math.max(1, fitting, squeezed)
}
