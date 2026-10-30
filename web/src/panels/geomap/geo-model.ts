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
  atlasOf,
  gazetteerName,
  resolveLookup,
  type Gazetteers,
  type LookupState,
} from "./gazetteer/lookup"
import { dayNightModel, type DayNightModel } from "./layers/day-night"
import {
  geojsonModel,
  geojsonUrlUsable,
  tooLargeNote,
  type DrawnFeature,
  type Feature,
  type RemoteGeojson,
} from "./layers/geojson"
import { heatmapModel, type HeatmapModel, type HeatRow } from "./layers/heatmap"
import {
  idText,
  networkModel,
  truncatedEdgesText,
  type EdgeRow,
  type NetworkModel,
  type NetworkNode,
} from "./layers/network"
import {
  photosModel,
  truncatedPhotosText,
  type PhotoRow,
  type PhotosModel,
} from "./layers/photos"
import { routeModel, type RouteModel, type RouteRow } from "./layers/route"
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
import type { AtlasName } from "./world"

export type Detail = { name: string; text: string }

export const markerSymbols = [
  "circle",
  "square",
  "triangle",
  "star",
  "cross",
  "x",
] as const

export type MarkerSymbol = (typeof markerSymbols)[number]

type Marker = {
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

export type Tip = Pick<Marker, "label" | "details" | "state" | "color">

type LayerBase = { name: string; opacity: number }
type Hoverable = { tooltip: boolean }

export type MarkersLayerModel = LayerBase &
  Hoverable & { kind: "markers"; textSize: number; markers: Marker[] }

type TilesLayerModel = LayerBase & { kind: "tiles"; source: TileSource }

export type HeatmapLayerModel = LayerBase & { kind: "heatmap" } & HeatmapModel

export type GeojsonLayerModel = LayerBase &
  Hoverable & {
    kind: "geojson"
    features: DrawnFeature[]
    corners: LonLat[]
  }

export type DayNightLayerModel = LayerBase & {
  kind: "dayNight"
} & DayNightModel

export type RouteLayerModel = LayerBase &
  Hoverable & { kind: "route" } & RouteModel

export type PhotosLayerModel = LayerBase &
  Hoverable & { kind: "photos" } & PhotosModel

export type NetworkLayerModel = LayerBase & { kind: "network" } & NetworkModel

type DrawnLayer =
  | MarkersLayerModel
  | TilesLayerModel
  | HeatmapLayerModel
  | GeojsonLayerModel
  | DayNightLayerModel
  | RouteLayerModel
  | PhotosLayerModel
  | NetworkLayerModel

export type GeoModel = {
  layers: DrawnLayer[]
  markers: Marker[]
  skipped: number
  truncated: number
  reason: string | null
  notes: string[]
  blocked: string[]
}

export type GeoContext = {
  gazetteers?: Gazetteers
  atlases?: Partial<Record<AtlasName, Feature[]>>
  remote?: Record<string, RemoteGeojson>
  time?: number
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
  | { kind: "lookup"; field: PreparedField; lookup: LookupState }

type Located = {
  frame: PreparedFrame
  frameIndex: number
  source: Source
  row: number
  lat: number
  lon: number
  place: string | null
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

function lookupSource(
  frame: PreparedFrame,
  layer: Layer,
  gazetteers: Gazetteers
): Source | null {
  const field =
    explicit(frame, layer.lookupField) ??
    frame.fields.find((candidate) => candidate.type === "string")
  if (!field) return null
  return {
    kind: "lookup",
    field,
    lookup: resolveLookup(gazetteerName(layer.gazetteer), gazetteers),
  }
}

function sourceOf(
  frame: PreparedFrame,
  layer: Layer,
  gazetteers: Gazetteers
): Source | null {
  if (layer.location === "coords") return coordsSource(frame, layer)
  if (layer.location === "geohash") return geohashSource(frame, layer)
  if (layer.location === "lookup") return lookupSource(frame, layer, gazetteers)
  const prefersGeohash =
    textOf(layer.geohashField) !== undefined &&
    textOf(layer.latField) === undefined &&
    textOf(layer.lonField) === undefined
  const coords = coordsSource(frame, layer)
  const geohash = geohashSource(frame, layer)
  return prefersGeohash ? (geohash ?? coords) : (coords ?? geohash)
}

function locationFields(source: Source): PreparedField[] {
  if (source.kind === "coords") return [source.lat, source.lon]
  return source.kind === "geohash" ? [source.geohash] : []
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

function pointOf(source: Source, row: number): LonLat | null {
  if (source.kind === "coords") {
    return {
      lat: coordinate(source.lat.values[row]),
      lon: coordinate(source.lon.values[row]),
    }
  }
  if (source.kind === "geohash") {
    const value = source.geohash.values[row]
    return typeof value === "string" ? decodeGeohash(value) : null
  }
  if (source.lookup.state !== "ready") return null
  return source.lookup.gazetteer.find(source.field.values[row])
}

function position(
  source: Source,
  row: number
): { lat: number; lon: number } | null {
  const point = pointOf(source, row)
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
  layer: Layer,
  gazetteers: Gazetteers
): { located: Located[]; skipped: number; found: boolean } {
  const located: Located[] = []
  const fixedColor = fixedColorOf(layer.color)
  let skipped = 0
  let found = false
  frames.forEach((frame, frameIndex) => {
    const source = sourceOf(frame, layer, gazetteers)
    if (!source) return
    found = true
    if (source.kind === "lookup" && source.lookup.state === "pending") return
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
        place:
          source.kind === "lookup"
            ? String(source.field.values[row] ?? "")
            : null,
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

function fixedColorOf(color: unknown): string | undefined {
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

function labelOf(item: Located): string {
  return item.place ?? `${item.lat.toFixed(3)}, ${item.lon.toFixed(3)}`
}

function plainTip(item: Located, fields: FieldResolver, color: string): Tip {
  return {
    label: labelOf(item),
    color,
    state: null,
    details: detailsOf(
      item.frame,
      fields,
      locationFields(item.source),
      item.row
    ),
  }
}

type Gathered = {
  all: Located[]
  kept: Located[]
  skipped: number
  truncated: number
  found: boolean
}

function gather(
  frames: PreparedFrame[],
  layer: Layer,
  gazetteers: Gazetteers,
  limit: number
): Gathered {
  const query = textOf(layer.query)
  const own =
    query === undefined ? frames : frames.filter((frame) => frame.ref === query)
  const { located, skipped, found } = locate(own, layer, gazetteers)
  return {
    all: located,
    kept: located.slice(0, limit),
    skipped,
    truncated: Math.max(0, located.length - limit),
    found,
  }
}

function markersFrom(
  gathered: Gathered,
  layer: Layer,
  fields: FieldResolver
): Marker[] {
  const radiusOf = sizeScale(gathered.all, layer)
  const fixed = fixedColorOf(layer.color)
  const symbol = symbolOf(layer.symbol)
  const markers = gathered.kept.map((item): Marker => {
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
      label: labelOf(item),
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
  return markers
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

function layerConfigs(display: Record<string, unknown>): Layer[] {
  return Array.isArray(display.layers)
    ? display.layers.filter(isRecord)
    : [flatLayer(display)]
}

export function layerNeeds(display: Record<string, unknown>): {
  atlases: AtlasName[]
  urls: string[]
} {
  const atlases = new Set<AtlasName>()
  const urls = new Set<string>()
  for (const config of layerConfigs(display)) {
    if (config.type === "geojson") {
      const url = textOf(config.url)
      if (url !== undefined) {
        if (geojsonUrlUsable(url)) urls.add(url)
      } else if (
        config.source === "countries" ||
        config.source === "usa-states"
      ) {
        atlases.add(config.source)
      }
    }
    const atlas = atlasOf(gazetteerName(config.gazetteer))
    if (config.location === "lookup" && atlas !== null) atlases.add(atlas)
  }
  return { atlases: [...atlases], urls: [...urls] }
}

type Env = {
  frames: PreparedFrame[]
  fields: FieldResolver
  context: GeoContext
  gazetteers: Gazetteers
  budget: number
}

type Built = {
  layers: DrawnLayer[]
  notes: string[]
  skipped: number
  truncated: number
  used: number
  found: boolean
  blocked: string[]
}

function built(partial: Partial<Built>): Built {
  return {
    layers: [],
    notes: [],
    skipped: 0,
    truncated: 0,
    used: 0,
    found: false,
    blocked: [],
    ...partial,
  }
}

function lookupNotes(config: Layer, env: Env): string[] {
  if (config.location !== "lookup") return []
  const state = resolveLookup(gazetteerName(config.gazetteer), env.gazetteers)
  return state.state === "unavailable" ? [state.note] : []
}

function locatedPart(config: Layer, env: Env, gathered: Gathered) {
  return {
    notes: lookupNotes(config, env),
    skipped: gathered.skipped,
    truncated: gathered.truncated,
    used: gathered.kept.length,
    found: gathered.found,
  }
}

function common(config: Layer, name: string) {
  return {
    name,
    opacity: layerOpacity(config),
    tooltip: config.tooltip !== false,
  }
}

function buildMarkers(config: Layer, name: string, env: Env): Built {
  const gathered = gather(env.frames, config, env.gazetteers, env.budget)
  return built({
    ...locatedPart(config, env, gathered),
    layers: [
      {
        kind: "markers",
        ...common(config, name),
        textSize: numberOf(config.textSize, defaultTextSize),
        markers: markersFrom(gathered, config, env.fields),
      },
    ],
  })
}

function memoByFrame(
  resolve: (frame: PreparedFrame) => PreparedField | undefined
): (frame: PreparedFrame) => PreparedField | undefined {
  const cache = new Map<PreparedFrame, PreparedField | undefined>()
  return (frame) => {
    if (!cache.has(frame)) cache.set(frame, resolve(frame))
    return cache.get(frame)
  }
}

function buildHeatmap(config: Layer, name: string, env: Env): Built {
  const gathered = gather(env.frames, config, env.gazetteers, env.budget)
  const weighted = textOf(config.weightField) !== undefined
  const weightField = memoByFrame((frame) =>
    explicit(frame, config.weightField)
  )
  const rows = gathered.kept.map((item): HeatRow => {
    const raw = weighted
      ? coordinate(weightField(item.frame)?.values[item.row])
      : 1
    return {
      frameIndex: item.frameIndex,
      row: item.row,
      lat: item.lat,
      lon: item.lon,
      weight: Number.isFinite(raw) ? raw : 0,
    }
  })
  return built({
    ...locatedPart(config, env, gathered),
    layers: [
      {
        kind: "heatmap",
        name,
        opacity: layerOpacity(config),
        ...heatmapModel(rows, config),
      },
    ],
  })
}

function buildRoute(config: Layer, name: string, env: Env): Built {
  const gathered = gather(env.frames, config, env.gazetteers, env.budget)
  const fixed = fixedColorOf(config.color)
  const timeField = memoByFrame((frame) =>
    frame.fields.find((field) => field.type === "time")
  )
  const rows = gathered.kept.map((item): RouteRow => {
    const time = timeField(item.frame)?.values[item.row]
    return {
      frameIndex: item.frameIndex,
      row: item.row,
      lat: item.lat,
      lon: item.lon,
      time: isNumber(time) ? time : null,
      tip: plainTip(
        item,
        env.fields,
        fixed ?? styleForSlot(item.frameIndex).color
      ),
    }
  })
  return built({
    ...locatedPart(config, env, gathered),
    layers: [
      {
        kind: "route",
        ...common(config, name),
        ...routeModel(rows, config, fixed),
      },
    ],
  })
}

function buildPhotos(config: Layer, name: string, env: Env): Built {
  const gathered = gather(env.frames, config, env.gazetteers, env.budget)
  const imageField = memoByFrame((frame) => explicit(frame, config.imageField))
  const fixed = fixedColorOf(config.color)
  const rows = gathered.kept.map((item): PhotoRow => ({
    frameIndex: item.frameIndex,
    row: item.row,
    lat: item.lat,
    lon: item.lon,
    url: imageField(item.frame)?.values[item.row],
    tip: plainTip(item, env.fields, fixed ?? styleForSlot(0).color),
  }))
  const model = photosModel(rows, config)
  const part = locatedPart(config, env, gathered)
  return built({
    ...part,
    notes: [
      ...part.notes,
      ...(textOf(config.imageField) === undefined
        ? [`Layer ${name} needs an image field.`]
        : []),
      ...(model.truncated > 0 ? [truncatedPhotosText(model.truncated)] : []),
    ],
    layers: [{ kind: "photos", ...common(config, name), ...model }],
  })
}

function hasField(frame: PreparedFrame, name: string): boolean {
  return frame.fields.some((field) => field.name === name)
}

function buildNetwork(config: Layer, name: string, env: Env): Built {
  const query = textOf(config.query)
  const own =
    query === undefined
      ? env.frames
      : env.frames.filter((frame) => frame.ref === query)
  const nodeFrames = own.filter((frame) => hasField(frame, "id"))
  const edgeFrames = own.filter(
    (frame) => hasField(frame, "source") && hasField(frame, "target")
  )
  const gathered = gather(nodeFrames, config, env.gazetteers, env.budget)
  const nodes = gathered.all.flatMap((item): NetworkNode[] => {
    const id = idText(
      item.frame.fields.find((field) => field.name === "id")?.values[item.row]
    )
    return id === null ? [] : [{ id, lat: item.lat, lon: item.lon }]
  })
  const edgeRows = edgeFrames.flatMap((frame, frameIndex) => {
    const source = frame.fields.find((field) => field.name === "source")
    const target = frame.fields.find((field) => field.name === "target")
    return Array.from({ length: rowsOf(frame) }, (_, row): EdgeRow => ({
      frameIndex,
      row,
      source: source?.values[row],
      target: target?.values[row],
    }))
  })
  const model = networkModel(
    nodes,
    edgeRows,
    config,
    fixedColorOf(config.color)
  )
  const part = locatedPart(config, env, gathered)
  return built({
    ...part,
    skipped: part.skipped + model.unlocated,
    notes: [
      ...part.notes,
      ...(model.truncated > 0 ? [truncatedEdgesText(model.truncated)] : []),
    ],
    layers: [
      { kind: "network", name, opacity: layerOpacity(config), ...model },
      {
        kind: "markers",
        ...common(config, name),
        textSize: numberOf(config.textSize, defaultTextSize),
        markers: markersFrom(gathered, config, env.fields),
      },
    ],
  })
}

function geojsonSource(
  config: Layer,
  name: string,
  env: Env
): { features: readonly Feature[]; notes: string[]; blocked: string[] } {
  const none = { features: [], notes: [], blocked: [] }
  const url = textOf(config.url)
  if (url !== undefined) {
    if (!geojsonUrlUsable(url)) {
      return { ...none, notes: [`Layer ${name} needs an https URL.`] }
    }
    const remote = env.context.remote?.[url]
    if (remote?.state === "ready") return { ...none, features: remote.features }
    if (remote?.state === "blocked") return { ...none, blocked: [remote.host] }
    if (remote?.state === "failed") {
      return { ...none, notes: [`Layer ${name} could not load its GeoJSON.`] }
    }
    return none
  }
  if (config.source === "countries" || config.source === "usa-states") {
    return { ...none, features: env.context.atlases?.[config.source] ?? [] }
  }
  return { ...none, notes: [`Layer ${name} needs a URL or a bundled source.`] }
}

function buildGeojson(config: Layer, name: string, env: Env): Built {
  const { features, notes, blocked } = geojsonSource(config, name, env)
  const model = geojsonModel(features, config, fixedColorOf)
  return built({
    notes: model.tooLarge ? [...notes, tooLargeNote] : notes,
    blocked,
    layers: [
      {
        kind: "geojson",
        ...common(config, name),
        features: model.features,
        corners: model.corners,
      },
    ],
  })
}

function buildDayNight(config: Layer, name: string, env: Env): Built {
  const date = new Date(env.context.time ?? Date.now())
  return built({
    layers: [
      {
        kind: "dayNight",
        name,
        opacity: layerOpacity(config),
        ...dayNightModel(date, config, fixedColorOf(config.nightColor)),
      },
    ],
  })
}

function buildTiles(config: Layer, name: string, theme: AppTheme): Built {
  const source = tileSource(config, theme)
  if (!source) {
    return built({
      notes: [`Layer ${name} needs an https URL with {z}, {x} and {y}.`],
    })
  }
  return built({
    layers: [{ kind: "tiles", name, opacity: layerOpacity(config), source }],
  })
}

const locatingTypes = ["markers", "heatmap", "route", "photos", "network"]
const standaloneTypes = ["geojson", "dayNight"]

export function geoModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>,
  theme: AppTheme = "light",
  context: GeoContext = {}
): GeoModel {
  const layers: DrawnLayer[] = []
  const notes: string[] = []
  const blocked = new Set<string>()
  let skipped = 0
  let truncated = 0
  let locating = 0
  let found = false
  let standalone = false
  const env: Env = {
    frames,
    fields,
    context,
    gazetteers: context.gazetteers ?? {},
    budget: maxMarkers,
  }
  for (const config of layerConfigs(display)) {
    const type = textOf(config.type) ?? "layer"
    const name = textOf(config.name) ?? type
    let result: Built
    switch (type) {
      case "markers":
        result = buildMarkers(config, name, env)
        break
      case "heatmap":
        result = buildHeatmap(config, name, env)
        break
      case "route":
        result = buildRoute(config, name, env)
        break
      case "photos":
        result = buildPhotos(config, name, env)
        break
      case "network":
        result = buildNetwork(config, name, env)
        break
      case "geojson":
        result = buildGeojson(config, name, env)
        break
      case "dayNight":
        result = buildDayNight(config, name, env)
        break
      default:
        result = tileLayerTypes.includes(type)
          ? buildTiles(config, name, theme)
          : built({ notes: [`Layer ${name} is not a layer this map knows.`] })
    }
    if (locatingTypes.includes(type)) locating++
    standalone ||=
      standaloneTypes.includes(type) ||
      result.layers.some((layer) => layer.kind === "tiles")
    found ||= result.found
    env.budget -= result.used
    skipped += result.skipped
    truncated += result.truncated
    layers.push(...result.layers)
    notes.push(...result.notes)
    for (const host of result.blocked) blocked.add(host)
  }
  if (locating > 0 && !found && !standalone) {
    return {
      layers: [],
      markers: [],
      skipped: 0,
      truncated: 0,
      reason: noLocationReason,
      notes,
      blocked: [...blocked],
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
    blocked: [...blocked],
  }
}

type FitRow = LonLat & { frameIndex: number; row: number }

function fitOf(
  layer: DrawnLayer
): { rows: readonly FitRow[]; fixed: readonly LonLat[] } | null {
  switch (layer.kind) {
    case "markers":
      return { rows: layer.markers, fixed: [] }
    case "heatmap":
      return { rows: layer.points, fixed: [] }
    case "route":
      return { rows: layer.lines.flatMap((line) => line.points), fixed: [] }
    case "photos":
      return { rows: layer.photos, fixed: [] }
    case "geojson":
      return { rows: [], fixed: layer.corners }
    default:
      return null
  }
}

function lastRows(rows: readonly FitRow[]): FitRow[] {
  const last = new Map<number, FitRow>()
  for (const entry of rows) {
    const seen = last.get(entry.frameIndex)
    if (!seen || entry.row > seen.row) last.set(entry.frameIndex, entry)
  }
  return [...last.values()]
}

export function fitPoints(model: GeoModel, choice: FitChoice): LonLat[] {
  const fitting = model.layers.flatMap((layer) => {
    const fit = fitOf(layer)
    return fit ? [{ layer, fit }] : []
  })
  const named = fitting.filter(({ layer }) => layer.name === choice.layer)
  return (named.length > 0 ? named : fitting).flatMap(({ fit }) => [
    ...(choice.lastOnly ? lastRows(fit.rows) : fit.rows),
    ...fit.fixed,
  ])
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
