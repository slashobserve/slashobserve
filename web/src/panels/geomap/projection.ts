export type Point = { x: number; y: number }
export type LonLat = { lon: number; lat: number }
export type View = { center: Point; zoom: number }

export const maxLatitude = 85.0511
export const maxZoom = 18

const tileSize = 256
const fitPadding = 32
const fitMaxZoom = 6
const worldBounds: LonLat[] = [
  { lon: -180, lat: -58 },
  { lon: 180, lat: 80 },
]

export function project(lon: number, lat: number): Point {
  const clamped = Math.max(-maxLatitude, Math.min(maxLatitude, lat))
  const radians = (clamped * Math.PI) / 180
  return {
    x: (lon + 180) / 360,
    y: 0.5 - Math.log(Math.tan(Math.PI / 4 + radians / 2)) / (2 * Math.PI),
  }
}

export function worldSize(zoom: number): number {
  return tileSize * 2 ** zoom
}

function zoomFor(
  points: readonly LonLat[],
  width: number,
  height: number,
  padding: number
): View {
  const projected = points.map(({ lon, lat }) => project(lon, lat))
  const xs = projected.map((point) => point.x)
  const ys = projected.map((point) => point.y)
  const minX = Math.min(...xs)
  const maxX = Math.max(...xs)
  const minY = Math.min(...ys)
  const maxY = Math.max(...ys)
  const scale = Math.max(
    (maxX - minX) / Math.max(1, width - 2 * padding),
    (maxY - minY) / Math.max(1, height - 2 * padding)
  )
  return {
    center: { x: (minX + maxX) / 2, y: (minY + maxY) / 2 },
    zoom: scale === 0 ? Infinity : -Math.log2(scale * tileSize),
  }
}

export function worldView(width: number, height: number): View {
  return zoomFor(worldBounds, width, height, 0)
}

export function minZoom(width: number, height: number): number {
  return worldView(width, height).zoom
}

export function clampZoom(zoom: number, width: number, height: number): number {
  return Math.max(minZoom(width, height), Math.min(maxZoom, zoom))
}

export function fitBounds(
  points: readonly LonLat[],
  width: number,
  height: number
): View {
  if (points.length === 0) return worldView(width, height)
  const fitted = zoomFor(points, width, height, fitPadding)
  return {
    center: fitted.center,
    zoom: Math.max(minZoom(width, height), Math.min(fitMaxZoom, fitted.zoom)),
  }
}
