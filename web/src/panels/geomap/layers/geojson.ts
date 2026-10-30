import { ApiError, toApiError } from "@/api/errors"
import { assetGeojsonUrl, remoteHost } from "@/lib/remote-assets"

import type { Detail, Tip } from "../geo-model"
import { project, type LonLat, type Point } from "../projection"

export type Position = readonly number[]

export type Geometry =
  | { type: "Point"; coordinates: Position }
  | { type: "MultiPoint"; coordinates: readonly Position[] }
  | { type: "LineString"; coordinates: readonly Position[] }
  | { type: "MultiLineString"; coordinates: readonly (readonly Position[])[] }
  | { type: "Polygon"; coordinates: readonly (readonly Position[])[] }
  | {
      type: "MultiPolygon"
      coordinates: readonly (readonly (readonly Position[])[])[]
    }

export type Feature = {
  id?: string | number
  properties: Record<string, unknown>
  geometry: Geometry
}

type FeatureStyle = {
  fill: string | null
  fillOpacity: number
  stroke: string
  strokeOpacity: number
}

export type Bounds = { minX: number; minY: number; maxX: number; maxY: number }

export type DrawnFeature = {
  key: string
  path: string
  dots: Point[]
  bounds: Bounds
  style: FeatureStyle
  tip: Tip
}

export type GeojsonModel = {
  features: DrawnFeature[]
  corners: LonLat[]
  tooLarge: boolean
}

export type RemoteGeojson =
  | { state: "loading" }
  | { state: "ready"; features: Feature[] }
  | { state: "blocked"; host: string }
  | { state: "failed" }

type Layer = Record<string, unknown>
type Paint = (value: unknown) => string | undefined

type Rule = {
  property: string
  operation: string
  value: string | number
  color?: unknown
  opacity?: number
}

export const maxFeatures = 5_000
export const maxCoordinates = 200_000
export const tooLargeNote = "This GeoJSON is too large to draw."

const maxCollectionDepth = 4
const maxDetails = 12
const maxDetailText = 120
const precision = 1e7
const antimeridianJump = 180
const fullTurn = 360
const defaultStrokeOpacity = 0.6
const defaultFillOpacity = 0.5
const labelProperties = ["name", "title", "label"]
const geometryTypes = new Set([
  "Point",
  "MultiPoint",
  "LineString",
  "MultiLineString",
  "Polygon",
  "MultiPolygon",
])
const maxRetries = 2
const placeholderOrigin = "https://app.invalid"

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function isPosition(value: unknown): value is Position {
  return (
    Array.isArray(value) &&
    value.length >= 2 &&
    Number.isFinite(value[0]) &&
    Number.isFinite(value[1])
  )
}

function featureId(value: unknown): string | number | undefined {
  return typeof value === "string" || typeof value === "number"
    ? value
    : undefined
}

function collect(
  value: unknown,
  properties: Record<string, unknown>,
  id: string | number | undefined,
  out: Feature[],
  depth: number
) {
  if (!isRecord(value) || depth > maxCollectionDepth) return
  if (value.type === "FeatureCollection") {
    if (!Array.isArray(value.features)) return
    for (const member of value.features)
      collect(member, {}, undefined, out, depth + 1)
  } else if (value.type === "Feature") {
    collect(
      value.geometry,
      isRecord(value.properties) ? value.properties : {},
      featureId(value.id),
      out,
      depth + 1
    )
  } else if (value.type === "GeometryCollection") {
    if (!Array.isArray(value.geometries)) return
    for (const member of value.geometries) {
      collect(member, properties, id, out, depth + 1)
    }
  } else if (
    typeof value.type === "string" &&
    geometryTypes.has(value.type) &&
    Array.isArray(value.coordinates)
  ) {
    out.push({ id, properties, geometry: value as Geometry })
  }
}

export function parseGeojson(raw: unknown): Feature[] {
  const out: Feature[] = []
  collect(raw, {}, undefined, out, 0)
  return out
}

function numeric(value: unknown): number | null {
  if (typeof value === "number") return Number.isFinite(value) ? value : null
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : null
  }
  return null
}

function equals(actual: unknown, expected: string | number): boolean {
  if (typeof expected === "number") return numeric(actual) === expected
  return (
    (typeof actual === "string" ||
      typeof actual === "number" ||
      typeof actual === "boolean") &&
    String(actual) === expected
  )
}

export function ruleMatches(
  rule: Pick<Rule, "property" | "operation" | "value">,
  properties: Record<string, unknown>
): boolean {
  const actual = properties[rule.property]
  if (rule.operation === "eq") return equals(actual, rule.value)
  if (rule.operation === "neq") return !equals(actual, rule.value)
  const left = numeric(actual)
  const right = numeric(rule.value)
  if (left === null || right === null) return false
  switch (rule.operation) {
    case "lt":
      return left < right
    case "lte":
      return left <= right
    case "gt":
      return left > right
    case "gte":
      return left >= right
    default:
      return false
  }
}

function rulesOf(layer: Layer): Rule[] {
  if (!Array.isArray(layer.rules)) return []
  return layer.rules.filter(
    (rule): rule is Rule =>
      isRecord(rule) &&
      typeof rule.property === "string" &&
      typeof rule.operation === "string" &&
      (typeof rule.value === "string" || typeof rule.value === "number")
  )
}

function styleOf(rule: Rule | undefined, paint: Paint): FeatureStyle {
  const color = rule?.color === undefined ? undefined : paint(rule.color)
  const opacity = typeof rule?.opacity === "number" ? rule.opacity : undefined
  if (color !== undefined) {
    return {
      fill: color,
      fillOpacity: opacity ?? defaultFillOpacity,
      stroke: color,
      strokeOpacity: 1,
    }
  }
  return {
    fill: null,
    fillOpacity: 0,
    stroke: "var(--foreground)",
    strokeOpacity: opacity ?? defaultStrokeOpacity,
  }
}

function round(value: number): number {
  return Math.round(value * precision) / precision
}

function detailText(value: unknown): string {
  const text =
    typeof value === "string" ? value : (JSON.stringify(value) ?? String(value))
  return text.length > maxDetailText
    ? `${text.slice(0, maxDetailText)}...`
    : text
}

function tipOf(feature: Feature, index: number, color: string): Tip {
  const entries = Object.entries(feature.properties)
  const named = entries.find(
    ([key, value]) =>
      labelProperties.includes(key.toLowerCase()) &&
      typeof value === "string" &&
      value !== ""
  )
  const details: Detail[] = entries
    .slice(0, maxDetails)
    .map(([name, value]) => ({ name, text: detailText(value) }))
  return {
    label: named ? String(named[1]) : `Feature ${index + 1}`,
    details,
    color,
    state: null,
  }
}

type Parts = {
  lines: Position[][]
  closed: boolean
  points: Position[]
}

function partsOf(geometry: Geometry): Parts {
  switch (geometry.type) {
    case "Point":
      return { lines: [], closed: false, points: [geometry.coordinates] }
    case "MultiPoint":
      return { lines: [], closed: false, points: [...geometry.coordinates] }
    case "LineString":
      return { lines: [[...geometry.coordinates]], closed: false, points: [] }
    case "MultiLineString":
      return {
        lines: geometry.coordinates.map((line) => [...line]),
        closed: false,
        points: [],
      }
    case "Polygon":
      return {
        lines: geometry.coordinates.map((ring) => [...ring]),
        closed: true,
        points: [],
      }
    case "MultiPolygon":
      return {
        lines: geometry.coordinates.flatMap((polygon) =>
          polygon.map((ring) => [...ring])
        ),
        closed: true,
        points: [],
      }
  }
}

type Extent = { west: number; south: number; east: number; north: number }

function emptyExtent(): Extent {
  return { west: Infinity, south: Infinity, east: -Infinity, north: -Infinity }
}

function grow(extent: Extent, lon: number, lat: number) {
  extent.west = Math.min(extent.west, lon)
  extent.east = Math.max(extent.east, lon)
  extent.south = Math.min(extent.south, lat)
  extent.north = Math.max(extent.north, lat)
}

function boundsOf(extent: Extent): Bounds {
  const low = project(extent.west, extent.north)
  const high = project(extent.east, extent.south)
  return { minX: low.x, minY: low.y, maxX: high.x, maxY: high.y }
}

export function unwrapRing(ring: readonly Position[]): Position[] {
  let offset = 0
  let previous: number | null = null
  return ring.map(([lon, lat]) => {
    if (previous !== null) {
      const step = lon - previous
      if (step > antimeridianJump) offset -= fullTurn
      else if (step < -antimeridianJump) offset += fullTurn
    }
    previous = lon
    return [lon + offset, lat]
  })
}

function clampLon(lon: number): number {
  return Math.max(-antimeridianJump, Math.min(antimeridianJump, lon))
}

function openPath(line: readonly Position[], extent: Extent): string {
  let path = ""
  let previous: number | null = null
  for (const position of line) {
    if (!isPosition(position)) continue
    const [lon, lat] = position
    grow(extent, lon, lat)
    const { x, y } = project(lon, lat)
    const breaks =
      previous === null || Math.abs(lon - previous) > antimeridianJump
    path += `${breaks ? "M" : "L"}${round(x)} ${round(y)}`
    previous = lon
  }
  return path
}

function ringPath(ring: readonly Position[], extent: Extent): string {
  const flat = unwrapRing(ring.filter(isPosition))
  if (flat.length === 0) return ""
  const lons = flat.map(([lon]) => lon)
  const east = Math.max(...lons)
  const west = Math.min(...lons)
  const wraps = east > antimeridianJump || west < -antimeridianJump
  const shifts = wraps
    ? [0, east > antimeridianJump ? -fullTurn : fullTurn]
    : [0]
  for (const [lon, lat] of flat) grow(extent, clampLon(lon), lat)
  if (wraps) {
    grow(extent, -antimeridianJump, flat[0][1])
    grow(extent, antimeridianJump, flat[0][1])
  }
  return shifts
    .map((shift) => {
      const steps = flat.map(([lon, lat], index) => {
        const { x, y } = project(lon + shift, lat)
        return `${index === 0 ? "M" : "L"}${round(x)} ${round(y)}`
      })
      return `${steps.join("")}Z`
    })
    .join("")
}

function countOf(parts: Parts): number {
  return parts.points.length + parts.lines.reduce((sum, l) => sum + l.length, 0)
}

export function geojsonModel(
  features: readonly Feature[],
  layer: Layer,
  paint: Paint
): GeojsonModel {
  const tooLarge: GeojsonModel = { features: [], corners: [], tooLarge: true }
  if (features.length > maxFeatures) return tooLarge
  const rules = rulesOf(layer)
  const drawn: DrawnFeature[] = []
  const layerExtent = emptyExtent()
  let coordinates = 0
  for (const [index, feature] of features.entries()) {
    const parts = partsOf(feature.geometry)
    coordinates += countOf(parts)
    if (coordinates > maxCoordinates) return tooLarge
    const extent = emptyExtent()
    const dots: Point[] = []
    for (const position of parts.points) {
      if (!isPosition(position)) continue
      grow(extent, position[0], position[1])
      dots.push(project(position[0], position[1]))
    }
    const path = parts.lines
      .map((line) =>
        parts.closed ? ringPath(line, extent) : openPath(line, extent)
      )
      .join("")
    if (path === "" && dots.length === 0) continue
    grow(layerExtent, extent.west, extent.south)
    grow(layerExtent, extent.east, extent.north)
    const style = styleOf(
      rules.find((rule) => ruleMatches(rule, feature.properties)),
      paint
    )
    drawn.push({
      key: String(index),
      path,
      dots,
      bounds: boundsOf(extent),
      style,
      tip: tipOf(feature, index, style.stroke),
    })
  }
  return {
    features: drawn,
    corners: Number.isFinite(layerExtent.west)
      ? [
          { lon: layerExtent.west, lat: layerExtent.south },
          { lon: layerExtent.east, lat: layerExtent.north },
        ]
      : [],
    tooLarge: false,
  }
}

export function geojsonUrlUsable(raw: string): boolean {
  return assetGeojsonUrl(raw, placeholderOrigin) !== null
}

export function retryGeojson(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
    return false
  }
  return failureCount < maxRetries
}

export async function fetchGeojson(
  raw: string,
  signal: AbortSignal
): Promise<Feature[]> {
  const href = assetGeojsonUrl(raw)
  if (href === null) {
    throw new ApiError(
      400,
      "invalid_asset_url",
      "The GeoJSON URL is not valid."
    )
  }
  const response = await fetch(href, {
    signal,
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  })
  const body: unknown = await response.json().catch(() => null)
  if (!response.ok) throw toApiError(response, body)
  return parseGeojson(body)
}

export function remoteGeojson(
  query: {
    status: "pending" | "success" | "error"
    data: Feature[] | undefined
    error: unknown
  },
  url: string
): RemoteGeojson {
  if (query.status === "success" && query.data) {
    return { state: "ready", features: query.data }
  }
  if (query.status !== "error") return { state: "loading" }
  const host = remoteHost(url, placeholderOrigin)
  if (
    query.error instanceof ApiError &&
    query.error.code === "outbound_domain_not_allowed" &&
    host !== null
  ) {
    return { state: "blocked", host }
  }
  return { state: "failed" }
}
