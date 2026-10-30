import { formatDateTime } from "@/lib/time"

import { styleForSlot } from "../colors"
import { displayValue, type FieldResolver } from "../field-config"
import { isNumber } from "../reducers"
import { thresholdColor, type ThresholdLevel } from "../thresholds"
import {
  matchesName,
  type PreparedField,
  type PreparedFrame,
} from "../transforms"
import {
  tileLayerTypes,
  tileSource,
  type AppTheme,
  type TileSource,
} from "./basemaps"
import { decodeGeohash } from "./geohash"
import {
  clampZoom,
  fitBounds,
  project,
  worldView,
  type LonLat,
  type View,
  type ZoomLimits,
} from "./projection"
import { isRegion, regionCorners } from "./regions"

type Detail = { name: string; text: string }

export const markerSymbols = [
  "circle",
  "square",
  "triangle",
  "star",
  "cross",
  "x",
] as const

export type MarkerSymbol = (typeof markerSymbols)[number]

export type Marker = {
  key: string
  frameIndex: number
  row: number
  lat: number
  lon: number
  x: number
  y: number
  radius: number
  color: string
  symbol: MarkerSymbol
  rotation: number
  text: string | null
  label: string
  details: Detail[]
  state: { level: Exclude<ThresholdLevel, "base">; word: string } | null
}

export type MarkersLayerModel = {
  kind: "markers"
  name: string
  opacity: number
  tooltip: boolean
  textSize: number
  markers: Marker[]
}

type TilesLayerModel = {
  kind: "tiles"
  name: string
  opacity: number
  source: TileSource
}

type DrawnLayer = MarkersLayerModel | TilesLayerModel

export type GeoModel = {
  layers: DrawnLayer[]
  markers: Marker[]
  skipped: number
  truncated: number
  reason: string | null
  notes: string[]
}

export type ViewOptions = {
  mode: string
  lat: number
  lon: number
  zoom: number
}

export type ViewTuning = {
  limits?: ZoomLimits
  paddingPercent?: number
}

export type FitChoice = { layer?: string; lastOnly: boolean }

type Layer = Record<string, unknown>

export const maxMarkers = 10_000
export const noLocationReason =
  "A geomap needs latitude and longitude fields, or a geohash field."

const defaultMarkerSize = 5
const defaultMarkerSizeMax = 15
const defaultZoom = 2
const defaultTextSize = 12
const markerKeys = [
  "location",
  "latField",
  "lonField",
  "geohashField",
  "sizeField",
  "colorField",
  "markerSize",
  "markerSizeMax",
] as const
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
  textField: PreparedField | undefined
  rotationField: PreparedField | undefined
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
    mode:
      mode === "world" || mode === "coords" || isRegion(mode) ? mode : "fit",
    lat: numberOf(display.lat, 0),
    lon: numberOf(display.lon, 0),
    zoom: numberOf(display.zoom, defaultZoom),
  }
}

export function viewTuning(display: Record<string, unknown>): ViewTuning {
  const limits: ZoomLimits = {}
  if (isNumber(display.minZoom)) limits.min = display.minZoom
  if (isNumber(display.maxZoom)) limits.max = display.maxZoom
  return {
    limits,
    ...(isNumber(display.fitPadding)
      ? { paddingPercent: display.fitPadding }
      : {}),
  }
}

export function fitChoice(display: Record<string, unknown>): FitChoice {
  return {
    layer: textOf(display.fitLayer),
    lastOnly: display.fitLastOnly === true,
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

function coordsSource(frame: PreparedFrame, layer: Layer): Source | null {
  const lat = explicit(frame, layer.latField) ?? named(frame, latitudeNames)
  const lon = explicit(frame, layer.lonField) ?? named(frame, longitudeNames)
  return lat && lon ? { kind: "coords", lat, lon } : null
}

function geohashSource(frame: PreparedFrame, layer: Layer): Source | null {
  const geohash =
    explicit(frame, layer.geohashField) ?? named(frame, geohashNames)
  return geohash ? { kind: "geohash", geohash } : null
}

function sourceOf(frame: PreparedFrame, layer: Layer): Source | null {
  if (layer.location === "coords") return coordsSource(frame, layer)
  if (layer.location === "geohash") return geohashSource(frame, layer)
  const prefersGeohash =
    textOf(layer.geohashField) !== undefined &&
    textOf(layer.latField) === undefined &&
    textOf(layer.lonField) === undefined
  const coords = coordsSource(frame, layer)
  const geohash = geohashSource(frame, layer)
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
  layer: Layer
): { located: Located[]; skipped: number; found: boolean } {
  const located: Located[] = []
  const fixedColor = fixedColorOf(layer)
  let skipped = 0
  let found = false
  frames.forEach((frame, frameIndex) => {
    const source = sourceOf(frame, layer)
    if (!source) return
    found = true
    const taken = locationFields(source)
    const sizeField = sideField(frame, layer.sizeField, taken)
    const colorField =
      explicit(frame, layer.colorField) ??
      (fixedColor === undefined ? firstNumber(frame, taken) : undefined)
    const textField = explicit(frame, layer.textField)
    const rotationField = explicit(frame, layer.rotationField)
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
        textField,
        rotationField,
      })
    }
  })
  return { located, skipped, found }
}

function sizeScale(located: Located[], layer: Layer) {
  const small = numberOf(layer.markerSize, defaultMarkerSize)
  const large = Math.max(
    small,
    numberOf(layer.markerSizeMax, defaultMarkerSizeMax)
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

function fieldText(
  frame: PreparedFrame,
  field: PreparedField,
  fields: FieldResolver,
  row: number
): string {
  const value = field.values[row]
  return field.type === "time" && isNumber(value)
    ? formatDateTime(value)
    : displayValue(value, fields.configFor(frame, field)).text
}

function detailsOf(
  frame: PreparedFrame,
  fields: FieldResolver,
  hidden: readonly PreparedField[],
  row: number
): Detail[] {
  return frame.fields.flatMap((field) => {
    if (hidden.includes(field)) return []
    if (fields.configFor(frame, field).hidden) return []
    return [
      {
        name: fields.displayName(frame, field),
        text: fieldText(frame, field, fields, row),
      },
    ]
  })
}

const fixedColorPattern = /^(?:ok|info|warning|critical|#[0-9a-fA-F]{6})$/

function fixedColorOf(layer: Layer): string | undefined {
  const color = layer.color
  return typeof color === "string" && fixedColorPattern.test(color)
    ? thresholdColor(color)
    : undefined
}

function colorOf(
  item: Located,
  fields: FieldResolver,
  fixed: string | undefined
): Pick<Marker, "color" | "state"> {
  const { frame, colorField, row } = item
  const fallback = fixed ?? styleForSlot(0).color
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

function symbolOf(value: unknown): MarkerSymbol {
  return markerSymbols.find((symbol) => symbol === value) ?? "circle"
}

function markersLayer(
  frames: PreparedFrame[],
  layer: Layer,
  fields: FieldResolver,
  limit = maxMarkers
): { markers: Marker[]; skipped: number; truncated: number; found: boolean } {
  const query = textOf(layer.query)
  const own =
    query === undefined ? frames : frames.filter((frame) => frame.ref === query)
  const { located, skipped, found } = locate(own, layer)
  const radiusOf = sizeScale(located, layer)
  const fixed = fixedColorOf(layer)
  const symbol = symbolOf(layer.symbol)
  const markers = located.slice(0, limit).map((item): Marker => {
    const { x, y } = project(item.lon, item.lat)
    const rotation = item.rotationField?.values[item.row]
    return {
      key: `${item.frameIndex}:${item.row}`,
      frameIndex: item.frameIndex,
      row: item.row,
      lat: item.lat,
      lon: item.lon,
      x,
      y,
      radius: radiusOf(item.size),
      symbol,
      rotation: isNumber(rotation) ? rotation : 0,
      text: item.textField
        ? fieldText(item.frame, item.textField, fields, item.row)
        : null,
      label: `${item.lat.toFixed(3)}, ${item.lon.toFixed(3)}`,
      details: detailsOf(
        item.frame,
        fields,
        locationFields(item.source),
        item.row
      ),
      ...colorOf(item, fields, fixed),
    }
  })
  markers.sort((a, b) => b.radius - a.radius)
  return {
    markers,
    skipped,
    truncated: Math.max(0, located.length - limit),
    found,
  }
}

function isRecord(value: unknown): value is Layer {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function flatLayer(display: Record<string, unknown>): Layer {
  const layer: Layer = { type: "markers", name: "markers" }
  for (const key of markerKeys) layer[key] = display[key]
  return layer
}

function layerOpacity(layer: Layer): number {
  return isNumber(layer.opacity) ? Math.max(0, Math.min(1, layer.opacity)) : 1
}

export function geoModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>,
  theme: AppTheme = "light"
): GeoModel {
  const configs = Array.isArray(display.layers)
    ? display.layers.filter(isRecord)
    : [flatLayer(display)]
  const layers: DrawnLayer[] = []
  const notes: string[] = []
  let skipped = 0
  let truncated = 0
  let budget = maxMarkers
  let markerLayers = 0
  let located = false
  for (const config of configs) {
    const type = textOf(config.type) ?? "layer"
    const name = textOf(config.name) ?? type
    if (type === "markers") {
      markerLayers++
      const built = markersLayer(frames, config, fields, budget)
      located ||= built.found
      budget -= built.markers.length
      skipped += built.skipped
      truncated += built.truncated
      layers.push({
        kind: "markers",
        name,
        opacity: layerOpacity(config),
        tooltip: config.tooltip !== false,
        textSize: numberOf(config.textSize, defaultTextSize),
        markers: built.markers,
      })
    } else if (tileLayerTypes.includes(type)) {
      const source = tileSource(config, theme)
      if (source) {
        layers.push({
          kind: "tiles",
          name,
          opacity: layerOpacity(config),
          source,
        })
      } else {
        notes.push(`Layer ${name} needs an https URL with {z}, {x} and {y}.`)
      }
    } else {
      notes.push(`Layer ${name} is not drawn yet.`)
    }
  }
  const tiled = layers.some((layer) => layer.kind === "tiles")
  if (markerLayers > 0 && !located && !tiled) {
    return {
      layers: [],
      markers: [],
      skipped: 0,
      truncated: 0,
      reason: noLocationReason,
      notes,
    }
  }
  return {
    layers,
    markers: layers.flatMap((layer) =>
      layer.kind === "markers" ? layer.markers : []
    ),
    skipped,
    truncated,
    reason: null,
    notes,
  }
}

export function fitPoints(model: GeoModel, choice: FitChoice): LonLat[] {
  const markerLayers = model.layers.filter((layer) => layer.kind === "markers")
  const named = markerLayers.filter((layer) => layer.name === choice.layer)
  const chosen = named.length > 0 ? named : markerLayers
  return chosen.flatMap((layer) => {
    if (!choice.lastOnly) return layer.markers
    const last = new Map<number, Marker>()
    for (const marker of layer.markers) {
      const seen = last.get(marker.frameIndex)
      if (!seen || marker.row > seen.row) last.set(marker.frameIndex, marker)
    }
    return [...last.values()]
  })
}

export function initialView(
  options: ViewOptions,
  points: readonly LonLat[],
  width: number,
  height: number,
  { limits, paddingPercent }: ViewTuning = {}
): View {
  if (options.mode === "world") {
    const world = worldView(width, height)
    return {
      center: world.center,
      zoom: clampZoom(world.zoom, width, height, limits),
    }
  }
  if (options.mode === "coords") {
    return {
      center: project(options.lon, options.lat),
      zoom: clampZoom(options.zoom, width, height, limits),
    }
  }
  if (isRegion(options.mode)) {
    return fitBounds(regionCorners(options.mode), width, height, { limits })
  }
  return fitBounds(points, width, height, {
    limits,
    padding:
      paddingPercent === undefined
        ? undefined
        : (Math.min(width, height) * paddingPercent) / 100,
  })
}

const triangleHeight = Math.sqrt(3) / 2
const starPoints = 5
const starInnerRatio = 0.4
const crossArm = 0.3

function round(value: number): number {
  return Math.round(value * 1000) / 1000
}

function polygon(points: readonly (readonly [number, number])[]): string {
  const steps = points.map(
    ([x, y], index) => `${index === 0 ? "M" : "L"}${round(x)} ${round(y)}`
  )
  return `${steps.join("")}Z`
}

function crossPoints(r: number, turn: number): [number, number][] {
  const a = r * crossArm
  const outline: [number, number][] = [
    [-a, -r],
    [a, -r],
    [a, -a],
    [r, -a],
    [r, a],
    [a, a],
    [a, r],
    [-a, r],
    [-a, a],
    [-r, a],
    [-r, -a],
    [-a, -a],
  ]
  const cos = Math.cos(turn)
  const sin = Math.sin(turn)
  return outline.map(([x, y]) => [x * cos - y * sin, x * sin + y * cos])
}

export function symbolPath(symbol: MarkerSymbol, r: number): string {
  switch (symbol) {
    case "square":
      return polygon([
        [-r, -r],
        [r, -r],
        [r, r],
        [-r, r],
      ])
    case "triangle":
      return polygon([
        [0, -r],
        [triangleHeight * r, r / 2],
        [-triangleHeight * r, r / 2],
      ])
    case "star":
      return polygon(
        Array.from({ length: starPoints * 2 }, (_, index) => {
          const radius = index % 2 === 0 ? r : r * starInnerRatio
          const angle = (index * Math.PI) / starPoints - Math.PI / 2
          return [radius * Math.cos(angle), radius * Math.sin(angle)] as const
        })
      )
    case "cross":
      return polygon(crossPoints(r, 0))
    case "x":
      return polygon(crossPoints(r, Math.PI / 4))
    default: {
      const radius = round(r)
      return `M${-radius} 0A${radius} ${radius} 0 1 0 ${radius} 0A${radius} ${radius} 0 1 0 ${-radius} 0Z`
    }
  }
}
