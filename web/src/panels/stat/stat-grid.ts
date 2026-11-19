export const tileGap = 8
const tileMinWidth = 120
const tileSqueezedWidth = 96
const tileMinHeight = 72
const tileCompactHeight = 56
const valueBaseSize = 28
const valueFloorSize = 12
const valueCharWidth = 0.6

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

const wideTileMinWidth = 200

export function tileSize(
  width: number,
  height: number,
  across: number,
  count: number
): { width: number; height: number } {
  const rows = Math.max(1, Math.ceil(count / across))
  return {
    width: (width - tileGap * (across - 1)) / across,
    height: (height - tileGap * (rows - 1)) / rows,
  }
}

export function isWideTile(
  wideLayout: boolean | undefined,
  size: { width: number; height: number }
): boolean {
  return (
    wideLayout === true &&
    size.width >= wideTileMinWidth &&
    size.width >= size.height * 2
  )
}

export function fittedValueSize(text: string, available: number): number {
  if (text.length === 0) return valueBaseSize
  const fitting = Math.floor(available / (text.length * valueCharWidth))
  return Math.max(valueFloorSize, Math.min(valueBaseSize, fitting))
}

export function rowMinHeight(height: number, rows: number): number {
  const needs = (row: number) => rows * row + tileGap * (rows - 1)
  if (needs(tileMinHeight) <= height) return tileMinHeight
  return needs(tileCompactHeight) <= height ? tileCompactHeight : tileMinHeight
}
