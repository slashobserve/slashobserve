import { formatDateTime } from "@/lib/time"

import { styleForSlot } from "../colors"
import { displayValue, type FieldResolver } from "../field-config"
import { isNumber } from "../reducers"
import type { ThresholdLevel } from "../thresholds"
import {
  matchesName,
  type PreparedField,
  type PreparedFrame,
} from "../transforms"
import {
  clampZoom,
  fitBounds,
  project,
  worldView,
  type View,
} from "./projection"
import { decodeGeohash } from "./geohash"

type Detail = { name: string; text: string }

export type Marker = {
  key: string
  lat: number
  lon: number
  x: number
  y: number
  radius: number
  color: string
  label: string
  details: Detail[]
  state: { level: Exclude<ThresholdLevel, "base">; word: string } | null
}

export type GeoModel = {
  markers: Marker[]
  skipped: number
  truncated: number
  reason: string | null
}

type ViewMode = "fit" | "world" | "coords"

export type ViewOptions = {
  mode: ViewMode
  lat: number
  lon: number
  zoom: number
}

export const maxMarkers = 10_000
export const noLocationReason =
  "A geomap needs latitude and longitude fields, or a geohash field."

const defaultMarkerSize = 5
const defaultMarkerSizeMax = 15
const defaultZoom = 2
const latitudeNames = ["lat", "latitude"]
const longitudeNames = ["lon", "lng", "longitude"]
const geohashNames = ["geohash"]

type Source =
  | { kind: "coords"; lat: PreparedField; lon: PreparedField }
  | { kind: "geohash"; geohash: PreparedField }

type Located = {
  frame: PreparedFrame
  frameIndex: number
  source: Source
  row: number
  lat: number
  lon: number
  size: number | null
  sizeField: PreparedField | undefined
  colorField: PreparedField | undefined
}

function textOf(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined
}

function numberOf(value: unknown, fallback: number): number {
  return isNumber(value) ? value : fallback
}

export function viewOptions(display: Record<string, unknown>): ViewOptions {
  const mode = display.view
  return {
    mode: mode === "world" || mode === "coords" ? mode : "fit",
    lat: numberOf(display.lat, 0),
    lon: numberOf(display.lon, 0),
    zoom: numberOf(display.zoom, defaultZoom),
  }
}

export function skippedText(count: number): string {
  return `${count} ${count === 1 ? "row had" : "rows had"} no location.`
}

export function truncatedText(count: number): string {
  return `${count} more rows were left off the map.`
}

function named(
  frame: PreparedFrame,
  names: readonly string[]
): PreparedField | undefined {
  for (const name of names) {
    const found = frame.fields.find(
      (field) => field.name.toLowerCase() === name
    )
    if (found) return found
  }
  return undefined
}

function explicit(
  frame: PreparedFrame,
  name: unknown
): PreparedField | undefined {
  const wanted = textOf(name)
  if (wanted === undefined) return undefined
  return frame.fields.find((field) => matchesName(frame, field, wanted))
}

function coordsSource(
  frame: PreparedFrame,
  display: Record<string, unknown>
): Source | null {
  const lat = explicit(frame, display.latField) ?? named(frame, latitudeNames)
  const lon = explicit(frame, display.lonField) ?? named(frame, longitudeNames)
  return lat && lon ? { kind: "coords", lat, lon } : null
}

function geohashSource(
  frame: PreparedFrame,
  display: Record<string, unknown>
): Source | null {
  const geohash =
    explicit(frame, display.geohashField) ?? named(frame, geohashNames)
  return geohash ? { kind: "geohash", geohash } : null
}

function sourceOf(
  frame: PreparedFrame,
  display: Record<string, unknown>
): Source | null {
  if (display.location === "coords") return coordsSource(frame, display)
  if (display.location === "geohash") return geohashSource(frame, display)
  const prefersGeohash =
    textOf(display.geohashField) !== undefined &&
    textOf(display.latField) === undefined &&
    textOf(display.lonField) === undefined
  const coords = coordsSource(frame, display)
  const geohash = geohashSource(frame, display)
  return prefersGeohash ? (geohash ?? coords) : (coords ?? geohash)
}

function locationFields(source: Source): PreparedField[] {
  return source.kind === "coords" ? [source.lat, source.lon] : [source.geohash]
}

function firstNumber(
  frame: PreparedFrame,
  taken: readonly PreparedField[]
): PreparedField | undefined {
  return frame.fields.find(
    (field) => field.type === "number" && !taken.includes(field)
  )
}

function sideField(
  frame: PreparedFrame,
  name: unknown,
  taken: readonly PreparedField[]
): PreparedField | undefined {
  return explicit(frame, name) ?? firstNumber(frame, taken)
}

function coordinate(value: unknown): number {
  if (typeof value === "number") return value
  if (typeof value === "string" && value.trim() !== "") return Number(value)
  return NaN
}

function position(
  source: Source,
  row: number
): { lat: number; lon: number } | null {
  const point =
    source.kind === "coords"
      ? {
          lat: coordinate(source.lat.values[row]),
          lon: coordinate(source.lon.values[row]),
        }
      : typeof source.geohash.values[row] === "string"
        ? decodeGeohash(source.geohash.values[row] as string)
        : null
  if (!point) return null
  const { lat, lon } = point
  if (!Number.isFinite(lat) || !Number.isFinite(lon)) return null
  if (Math.abs(lat) > 90 || Math.abs(lon) > 180) return null
  return point
}

function rowsOf(frame: PreparedFrame): number {
  return Math.max(0, ...frame.fields.map((field) => field.values.length))
}

function locate(
  frames: PreparedFrame[],
  display: Record<string, unknown>
): { located: Located[]; skipped: number; found: boolean } {
  const located: Located[] = []
  let skipped = 0
  let found = false
  frames.forEach((frame, frameIndex) => {
    const source = sourceOf(frame, display)
    if (!source) return
    found = true
    const taken = locationFields(source)
    const sizeField = sideField(frame, display.sizeField, taken)
    const colorField = sideField(frame, display.colorField, taken)
    const rows = rowsOf(frame)
    for (let row = 0; row < rows; row++) {
      const point = position(source, row)
      if (!point) {
        skipped++
        continue
      }
      const raw = sizeField?.values[row]
      located.push({
        frame,
        frameIndex,
        source,
        row,
        ...point,
        size: isNumber(raw) ? raw : null,
        sizeField,
        colorField,
      })
    }
  })
  return { located, skipped, found }
}

function sizeScale(located: Located[], display: Record<string, unknown>) {
  const small = numberOf(display.markerSize, defaultMarkerSize)
  const large = Math.max(
    small,
    numberOf(display.markerSizeMax, defaultMarkerSizeMax)
  )
  let min = Infinity
  let max = -Infinity
  for (const { size } of located) {
    if (size === null) continue
    min = Math.min(min, size)
    max = Math.max(max, size)
  }
  return (size: number | null): number =>
    size === null || max <= min
      ? small
      : small + ((size - min) / (max - min)) * (large - small)
}

function detailsOf(
  frame: PreparedFrame,
  fields: FieldResolver,
  hidden: readonly PreparedField[],
  row: number
): Detail[] {
  return frame.fields.flatMap((field) => {
    if (hidden.includes(field)) return []
    const config = fields.configFor(frame, field)
    if (config.hidden) return []
    const value = field.values[row]
    const text =
      field.type === "time" && isNumber(value)
        ? formatDateTime(value)
        : displayValue(value, config).text
    return [{ name: fields.displayName(frame, field), text }]
  })
}

function colorOf(
  item: Located,
  fields: FieldResolver
): Pick<Marker, "color" | "state"> {
  const { frame, colorField, row } = item
  const fallback = styleForSlot(0).color
  if (!colorField) return { color: fallback, state: null }
  const shown = displayValue(
    colorField.values[row],
    fields.configFor(frame, colorField)
  )
  const fromThreshold =
    shown.threshold.level !== "base" &&
    shown.threshold.color !== null &&
    shown.threshold.color === shown.color
  return {
    color: shown.color ?? fallback,
    state: fromThreshold
      ? {
          level: shown.threshold.level as Exclude<ThresholdLevel, "base">,
          word: shown.threshold.label,
        }
      : null,
  }
}

export function geoModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>
): GeoModel {
  const { located, skipped, found } = locate(frames, display)
  if (!found) {
    return { markers: [], skipped: 0, truncated: 0, reason: noLocationReason }
  }
  const radiusOf = sizeScale(located, display)
  const markers = located.slice(0, maxMarkers).map((item): Marker => {
    const { x, y } = project(item.lon, item.lat)
    return {
      key: `${item.frameIndex}:${item.row}`,
      lat: item.lat,
      lon: item.lon,
      x,
      y,
      radius: radiusOf(item.size),
      label: `${item.lat.toFixed(3)}, ${item.lon.toFixed(3)}`,
      details: detailsOf(
        item.frame,
        fields,
        locationFields(item.source),
        item.row
      ),
      ...colorOf(item, fields),
    }
  })
  markers.sort((a, b) => b.radius - a.radius)
  return {
    markers,
    skipped,
    truncated: Math.max(0, located.length - maxMarkers),
    reason: null,
  }
}

export function initialView(
  options: ViewOptions,
  markers: readonly Marker[],
  width: number,
  height: number
): View {
  if (options.mode === "world") return worldView(width, height)
  if (options.mode === "coords") {
    return {
      center: project(options.lon, options.lat),
      zoom: clampZoom(options.zoom, width, height),
    }
  }
  return fitBounds(markers, width, height)
}
