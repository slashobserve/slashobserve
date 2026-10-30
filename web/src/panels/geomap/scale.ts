import { tileSize, type LonLat } from "./projection"

type ScaleBar = { label: string; pixels: number }
export type ScaleBars = { metric: ScaleBar; imperial: ScaleBar }

const earthRadius = 6_371_008.8
const equatorMeters = 40_075_016.686
const metersPerFoot = 0.3048
const metersPerMile = 1609.344
const niceSteps = [1, 2, 5]

export function metersPerPixel(latitude: number, zoom: number): number {
  const radians = (latitude * Math.PI) / 180
  return (equatorMeters * Math.cos(radians)) / (tileSize * 2 ** zoom)
}

export function niceLength(limit: number): number {
  if (!(limit > 0)) return 0
  const power = 10 ** Math.floor(Math.log10(limit))
  let best = power
  for (const step of niceSteps) {
    if (step * power <= limit) best = step * power
  }
  return best
}

function metricLabel(meters: number): string {
  return meters >= 1000 ? `${meters / 1000} km` : `${meters} m`
}

function imperialLabel(feet: number): string {
  return feet >= metersPerMile / metersPerFoot
    ? `${(feet * metersPerFoot) / metersPerMile} mi`
    : `${feet} ft`
}

export function scaleBars(
  latitude: number,
  zoom: number,
  maxPixels: number
): ScaleBars {
  const perPixel = metersPerPixel(latitude, zoom)
  const meters = niceLength(perPixel * maxPixels)
  const limitFeet = (perPixel * maxPixels) / metersPerFoot
  const mileFeet = metersPerMile / metersPerFoot
  const feet =
    limitFeet >= mileFeet
      ? niceLength(limitFeet / mileFeet) * mileFeet
      : niceLength(limitFeet)
  return {
    metric: { label: metricLabel(meters), pixels: meters / perPixel },
    imperial: {
      label: imperialLabel(feet),
      pixels: (feet * metersPerFoot) / perPixel,
    },
  }
}

export function distanceMeters(a: LonLat, b: LonLat): number {
  const toRadians = Math.PI / 180
  const dLat = (b.lat - a.lat) * toRadians
  const dLon = (b.lon - a.lon) * toRadians
  const h =
    Math.sin(dLat / 2) ** 2 +
    Math.cos(a.lat * toRadians) *
      Math.cos(b.lat * toRadians) *
      Math.sin(dLon / 2) ** 2
  return 2 * earthRadius * Math.asin(Math.min(1, Math.sqrt(h)))
}

export function pathMeters(points: readonly LonLat[]): number {
  let total = 0
  for (let i = 1; i < points.length; i++) {
    total += distanceMeters(points[i - 1], points[i])
  }
  return total
}

function trimmed(value: number, digits: number): string {
  return Number(value.toFixed(digits)).toLocaleString("en-US")
}

export function formatMetric(meters: number): string {
  if (meters < 1000) return `${trimmed(meters, 0)} m`
  return `${trimmed(meters / 1000, meters < 100_000 ? 2 : 0)} km`
}

export function formatImperial(meters: number): string {
  const feet = meters / metersPerFoot
  if (feet < 1000) return `${trimmed(feet, 0)} ft`
  const miles = meters / metersPerMile
  return `${trimmed(miles, miles < 100 ? 2 : 0)} mi`
}
