import { project, type LonLat, type Point } from "../projection"

export type DayNightModel = {
  path: string
  sun: Point | null
  color: string | null
}

const msPerDay = 86_400_000
const longitudeSamples = 360
const poleLatitude = 90
const precision = 1e7
const minDeclination = 1e-9

function degrees(radians: number): number {
  return (radians * 180) / Math.PI
}

function dayOfYear(date: Date): number {
  const start = Date.UTC(date.getUTCFullYear(), 0, 1)
  return Math.floor((date.getTime() - start) / msPerDay) + 1
}

function solarPosition(date: Date): {
  declination: number
  subsolarLon: number
} {
  const hours =
    date.getUTCHours() + date.getUTCMinutes() / 60 + date.getUTCSeconds() / 3600
  const year = (2 * Math.PI * (dayOfYear(date) - 1 + (hours - 12) / 24)) / 365
  const equationOfTime =
    229.18 *
    (0.000075 +
      0.001868 * Math.cos(year) -
      0.032077 * Math.sin(year) -
      0.014615 * Math.cos(2 * year) -
      0.040849 * Math.sin(2 * year))
  const declination =
    0.006918 -
    0.399912 * Math.cos(year) +
    0.070257 * Math.sin(year) -
    0.006758 * Math.cos(2 * year) +
    0.000907 * Math.sin(2 * year) -
    0.002697 * Math.cos(3 * year) +
    0.00148 * Math.sin(3 * year)
  return {
    declination,
    subsolarLon: -15 * (hours - 12 + equationOfTime / 60),
  }
}

function wrapLon(lon: number): number {
  return ((((lon + 180) % 360) + 360) % 360) - 180
}

export function subsolarPoint(date: Date): LonLat {
  const { declination, subsolarLon } = solarPosition(date)
  return { lat: degrees(declination), lon: wrapLon(subsolarLon) }
}

export function terminator(date: Date): LonLat[] {
  const { declination, subsolarLon } = solarPosition(date)
  const northern = declination >= 0
  const tilt =
    (northern ? 1 : -1) *
    Math.max(Math.abs(Math.tan(declination)), minDeclination)
  const pole = northern ? -poleLatitude : poleLatitude
  const edge: LonLat[] = []
  for (let step = 0; step <= longitudeSamples; step++) {
    const lon = -180 + (360 * step) / longitudeSamples
    const hourAngle = ((lon - subsolarLon) * Math.PI) / 180
    edge.push({ lon, lat: degrees(Math.atan(-Math.cos(hourAngle) / tilt)) })
  }
  return [{ lon: -180, lat: pole }, ...edge, { lon: 180, lat: pole }]
}

function round(value: number): number {
  return Math.round(value * precision) / precision
}

export function dayNightModel(
  date: Date,
  layer: Record<string, unknown>,
  fixed: string | undefined
): DayNightModel {
  const path = terminator(date)
    .map(({ lon, lat }, index) => {
      const { x, y } = project(lon, lat)
      return `${index === 0 ? "M" : "L"}${round(x)} ${round(y)}`
    })
    .join("")
  const sun = layer.showSun === true ? subsolarPoint(date) : null
  return {
    path: `${path}Z`,
    sun: sun ? project(sun.lon, sun.lat) : null,
    color: fixed ?? null,
  }
}
