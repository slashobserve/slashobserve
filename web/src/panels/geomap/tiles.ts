import { tileSize, worldSize, type View } from "./projection"

export type Tile = {
  z: number
  x: number
  column: number
  y: number
  left: number
  top: number
  size: number
}

export const maxTiles = 64
const maxTileZoom = 18

export type TileOptions = {
  minLevel?: number
  maxLevel?: number
  repeat?: boolean
}

function tilesAt(
  view: View,
  width: number,
  height: number,
  z: number,
  repeat: boolean
): Tile[] {
  const size = tileSize * 2 ** (view.zoom - z)
  const pixels = worldSize(view.zoom)
  const originLeft = width / 2 - view.center.x * pixels
  const originTop = height / 2 - view.center.y * pixels
  const columns = 2 ** z
  const firstX = repeat
    ? Math.floor(-originLeft / size)
    : Math.max(0, Math.floor(-originLeft / size))
  const lastX = repeat
    ? Math.ceil((width - originLeft) / size) - 1
    : Math.min(columns - 1, Math.ceil((width - originLeft) / size) - 1)
  const firstY = Math.max(0, Math.floor(-originTop / size))
  const lastY = Math.min(
    columns - 1,
    Math.ceil((height - originTop) / size) - 1
  )
  const tiles: Tile[] = []
  for (let y = firstY; y <= lastY; y++) {
    for (let x = firstX; x <= lastX; x++) {
      tiles.push({
        z,
        x: ((x % columns) + columns) % columns,
        column: x,
        y,
        left: originLeft + x * size,
        top: originTop + y * size,
        size,
      })
    }
  }
  return tiles
}

export function visibleTiles(
  view: View,
  width: number,
  height: number,
  { minLevel = 0, maxLevel = maxTileZoom, repeat = true }: TileOptions = {}
): Tile[] {
  let z = Math.max(minLevel, Math.min(maxLevel, Math.floor(view.zoom)))
  let tiles = tilesAt(view, width, height, z, repeat)
  while (tiles.length > maxTiles && z > minLevel) {
    z -= 1
    tiles = tilesAt(view, width, height, z, repeat)
  }
  return tiles.length > maxTiles ? [] : tiles
}
