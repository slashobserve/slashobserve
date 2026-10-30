export type Point = { x: number; y: number }
export type LonLat = { lon: number; lat: number }
export type View = { center: Point; zoom: number }

export type ZoomLimits = { min?: number; max?: number }
export type FitOptions = { padding?: number; limits?: ZoomLimits }

export const maxLatitude = 85.0511
export const maxZoom = 18

export const tileSize = 256
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

export function unproject(x: number, y: number): LonLat {
  const lat = Math.atan(Math.sinh(Math.PI * (1 - 2 * y)))
  return { lon: x * 360 - 180, lat: (lat * 180) / Math.PI }
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

export function zoomBounds(
  width: number,
  height: number,
  limits: ZoomLimits = {}
): { lowest: number; highest: number } {
  return {
    lowest: Math.max(minZoom(width, height), limits.min ?? -Infinity),
    highest: Math.min(maxZoom, limits.max ?? Infinity),
  }
}

export function clampZoom(
  zoom: number,
  width: number,
  height: number,
  limits: ZoomLimits = {}
): number {
  const { lowest, highest } = zoomBounds(width, height, limits)
  return Math.min(highest, Math.max(lowest, zoom))
}

export function fitBounds(
  points: readonly LonLat[],
  width: number,
  height: number,
  { padding = fitPadding, limits }: FitOptions = {}
): View {
  if (points.length === 0) {
    const world = worldView(width, height)
    return {
      center: world.center,
      zoom: clampZoom(world.zoom, width, height, limits),
    }
  }
  const fitted = zoomFor(points, width, height, padding)
  return {
    center: fitted.center,
    zoom: clampZoom(Math.min(fitMaxZoom, fitted.zoom), width, height, limits),
  }
}

export function zoomAround(
  view: View,
  zoom: number,
  pointerX: number,
  pointerY: number,
  width: number,
  height: number,
  limits: ZoomLimits = {}
): View {
  const next = clampZoom(zoom, width, height, limits)
  const before = worldSize(view.zoom)
  const after = worldSize(next)
  const offsetX = pointerX - width / 2
  const offsetY = pointerY - height / 2
  return {
    zoom: next,
    center: {
      x: clampCenter(view.center.x + offsetX / before - offsetX / after),
      y: clampCenter(view.center.y + offsetY / before - offsetY / after),
    },
  }
}

export function clampCenter(value: number): number {
  return Math.max(0, Math.min(1, value))
}

export function panBy(
  view: View,
  dx: number,
  dy: number,
  width: number,
  height: number
): View {
  const size = worldSize(view.zoom)
  return {
    zoom: view.zoom,
    center: {
      x: clampCenter(view.center.x + (dx * width) / size),
      y: clampCenter(view.center.y + (dy * height) / size),
    },
  }
}
