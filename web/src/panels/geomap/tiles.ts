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

function tilesAt(view: View, width: number, height: number, z: number): Tile[] {
  const size = tileSize * 2 ** (view.zoom - z)
  const pixels = worldSize(view.zoom)
  const originLeft = width / 2 - view.center.x * pixels
  const originTop = height / 2 - view.center.y * pixels
  const columns = 2 ** z
  const firstX = Math.floor(-originLeft / size)
  const lastX = Math.ceil((width - originLeft) / size) - 1
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
  maxLevel = maxTileZoom
): Tile[] {
  let z = Math.max(0, Math.min(maxLevel, Math.floor(view.zoom)))
  let tiles = tilesAt(view, width, height, z)
  while (tiles.length > maxTiles && z > 0) {
    z -= 1
    tiles = tilesAt(view, width, height, z)
  }
  return tiles
}
