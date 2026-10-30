import {
  canvasConnectionDefaults,
  canvasElementDefaults,
  type Reducer,
} from "@/spec"

import { styleForSlot, resolveColor } from "../colors"
import { cssColorToRgb, readableText, type Rgb } from "../contrast"
import {
  displayValue,
  fieldRange,
  type DisplayValue,
  type FieldResolver,
} from "../field-config"
import { reduce } from "../reducers"
import { thresholdColor, type ThresholdLevel } from "../thresholds"
import type { PreparedField, PreparedFrame } from "../transforms"
import {
  scaledWidth,
  type ArrowMode,
  type ConnectionPath,
  type LineStyle,
  type Point,
} from "./connections"
import {
  elementBox,
  parsePlacement,
  type Box,
  type Placement,
  type Size,
} from "./placement"

const kinds = [
  "text",
  "metric",
  "rectangle",
  "ellipse",
  "icon",
  "button",
  "server",
  "triangle",
  "cloud",
  "parallelogram",
  "windTurbine",
  "droneTop",
  "droneFront",
  "droneSide",
] as const

export type CanvasKind = (typeof kinds)[number]
export type CanvasAlign = "left" | "center" | "right"
export type CanvasValign = "top" | "middle" | "bottom"
export type ImageSize = "original" | "contain" | "cover" | "fill" | "tile"
export type ServerKind = "single" | "stack" | "database" | "terminal"
export type ButtonStyle = "primary" | "secondary" | "success" | "destructive"
export type RgbOf = (color: string) => Rgb | null

export type Background = { url: string; size: ImageSize }
export type Detail = { name: string; text: string }

export type MetricState = {
  level: ThresholdLevel
  label: string
  color: string
  labelColor: string | null
}

export type CanvasElement = {
  key: string
  id: string
  kind: CanvasKind
  box: Box
  text: string | null
  valueText: string | null
  fill: string | null
  stroke: string | null
  textColor: string | null
  fontSize: number
  align: CanvasAlign
  valign: CanvasValign
  rotation: number
  borderWidth: number
  radius: number
  ownRadius: number | null
  background: Background | null
  icon: string | null
  image: string | null
  iconColor: string | null
  server: ServerKind
  buttonStyle: ButtonStyle
  accent: string | null
  angle: number
  details: Detail[]
  state: MetricState | null
}

export type CanvasLine = {
  key: string
  x1: number
  y1: number
  x2: number
  y2: number
  points: Point[]
  path: ConnectionPath
  lineStyle: LineStyle
  arrow: ArrowMode
  color: string
  width: number
}

type CanvasRoot = {
  backgroundColor: string | null
  borderColor: string | null
  borderWidth: number
  borderRadius: number
  background: Background | null
  panZoom: boolean
  infinitePan: boolean
  tooltip: boolean
}

export type CanvasModel = {
  root: CanvasRoot
  elements: CanvasElement[]
  lines: CanvasLine[]
}

const noDataText = "No data"
export const defaultLineColor = "var(--muted-foreground)"
export const foregroundColor = "var(--foreground)"

const aligns = ["left", "center", "right"] as const
const valigns = ["top", "middle", "bottom"] as const
const imageSizes = ["original", "contain", "cover", "fill", "tile"] as const
const serverKinds = ["single", "stack", "database", "terminal"] as const
const buttonStyles = ["primary", "secondary", "success", "destructive"] as const
const paths = ["straight", "curved"] as const
const lineStyles = ["solid", "dashed", "dotted"] as const
const arrows = ["none", "forward", "reverse", "both"] as const
const defaultImageSize: ImageSize = "contain"
const safeColor =
  /^(?:#[0-9a-fA-F]{6}|var\(--(?:threshold-(?:ok|info|warning|critical)|series-[1-8])\))$/
const specColor = /^(?:ok|info|warning|critical|#[0-9a-fA-F]{6})$/

type Entry = {
  frame: PreparedFrame
  field: PreparedField
  seriesName: string
  displayName: string
  slotColor: string
  value: number | null
}

type Bound = { entry: Entry; shown: DisplayValue; color: string }

type ImageRef = { url: string } | { field: string }

type ColorRef = "field" | string | null

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function entriesOf(
  frames: PreparedFrame[],
  fields: FieldResolver,
  reducer: Reducer
): Entry[] {
  const entries: Entry[] = []
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (field.type !== "number") continue
      if (fields.configFor(frame, field).hidden) continue
      entries.push({
        frame,
        field,
        seriesName: fields.seriesName(frame, field),
        displayName: fields.displayName(frame, field),
        slotColor: styleForSlot(entries.length).color,
        value: reduce(field.values, reducer),
      })
    }
  }
  return entries
}

function finiteIn(value: unknown, min: number, max: number): number | null {
  return typeof value === "number" && Number.isFinite(value)
    ? Math.min(max, Math.max(min, value))
    : null
}

function oneOf<T extends string>(
  allowed: readonly T[],
  value: unknown
): T | null {
  return typeof value === "string" &&
    (allowed as readonly string[]).includes(value)
    ? (value as T)
    : null
}

function imageRef(raw: unknown): ImageRef | null {
  if (!isRecord(raw)) return null
  if (typeof raw.url === "string" && raw.url !== "") return { url: raw.url }
  if (typeof raw.field === "string" && raw.field !== "") {
    return { field: raw.field }
  }
  return null
}

function colorRef(value: unknown): ColorRef {
  if (value === "field") return "field"
  return typeof value === "string" && specColor.test(value) ? value : null
}

function cssColor(ref: ColorRef, bound: Bound | null): string | null {
  if (ref === null) return null
  if (ref === "field") return bound?.color ?? null
  return thresholdColor(ref)
}

function box(raw: Record<string, unknown>): Box | null {
  const left = finiteIn(raw.left, 0, 100)
  const top = finiteIn(raw.top, 0, 100)
  const width = finiteIn(raw.width, 0, 100)
  const height = finiteIn(raw.height, 0, 100)
  if (left === null || top === null || !width || !height) return null
  return { left, top, width, height }
}

function fieldName(value: unknown): string | null {
  return typeof value === "string" ? value : null
}

type Parsed = {
  id: string
  kind: CanvasKind
  percent: Box
  placement: Placement | null
  text: string | null
  field: string | null
  fill: ColorRef
  stroke: ColorRef
  textColor: ColorRef
  iconColor: ColorRef
  accentColor: ColorRef
  fontSize: number
  align: CanvasAlign
  valign: CanvasValign
  rotation: number
  borderWidth: number
  radius: number | null
  background: ImageRef | null
  backgroundSize: ImageSize
  icon: string | null
  image: ImageRef | null
  server: ServerKind
  buttonStyle: ButtonStyle
  rateField: string | null
  yawField: string | null
  pitchField: string | null
  rollField: string | null
}

function parseElement(raw: unknown): Parsed | null {
  if (!isRecord(raw)) return null
  const { id } = raw
  const kind = oneOf(kinds, raw.kind)
  if (typeof id !== "string" || kind === null) return null
  const percent = box(raw)
  if (!percent) return null
  return {
    id,
    kind,
    percent,
    placement: parsePlacement(raw.placement),
    text: typeof raw.text === "string" && raw.text !== "" ? raw.text : null,
    field: fieldName(raw.field),
    fill: colorRef(raw.fill),
    stroke: colorRef(raw.stroke),
    textColor: colorRef(raw.textColor),
    iconColor: colorRef(raw.iconColor),
    accentColor: colorRef(raw.accentColor),
    fontSize: finiteIn(raw.fontSize, 8, 72) ?? canvasElementDefaults.fontSize,
    align: oneOf(aligns, raw.align) ?? canvasElementDefaults.align,
    valign: oneOf(valigns, raw.valign) ?? canvasElementDefaults.valign,
    rotation:
      finiteIn(raw.rotation, -360, 360) ?? canvasElementDefaults.rotation,
    borderWidth:
      finiteIn(raw.borderWidth, 0, 20) ?? canvasElementDefaults.borderWidth,
    radius: finiteIn(raw.radius, 0, 100),
    background: imageRef(raw.backgroundImage),
    backgroundSize: oneOf(imageSizes, raw.backgroundSize) ?? defaultImageSize,
    icon: typeof raw.icon === "string" && raw.icon !== "" ? raw.icon : null,
    image: imageRef(raw.image),
    server: oneOf(serverKinds, raw.server) ?? "single",
    buttonStyle: oneOf(buttonStyles, raw.buttonStyle) ?? "primary",
    rateField: fieldName(raw.rateField),
    yawField: fieldName(raw.yawField),
    pitchField: fieldName(raw.pitchField),
    rollField: fieldName(raw.rollField),
  }
}

type Placed = { parsed: Parsed; box: Box }

function center(place: Box): [number, number] {
  return [place.left + place.width / 2, place.top + place.height / 2]
}

function edgePoint(
  place: Box,
  kind: CanvasKind,
  [tx, ty]: [number, number]
): [number, number] {
  const [cx, cy] = center(place)
  const dx = tx - cx
  const dy = ty - cy
  const reach =
    kind === "ellipse"
      ? Math.hypot(dx / place.width, dy / place.height)
      : Math.max(Math.abs(dx) / place.width, Math.abs(dy) / place.height)
  return reach === 0 ? [cx, cy] : [cx + dx / (2 * reach), cy + dy / (2 * reach)]
}

function verticesOf(raw: unknown, body: Size): [number, number][] {
  if (!Array.isArray(raw)) return []
  return raw.flatMap((vertex): [number, number][] => {
    if (!isRecord(vertex)) return []
    const x = finiteIn(vertex.x, 0, 100)
    const y = finiteIn(vertex.y, 0, 100)
    return x === null || y === null
      ? []
      : [[(x * body.width) / 100, (y * body.height) / 100]]
  })
}

function widthOf(
  raw: Record<string, unknown>,
  bind: (name: string | null) => Bound | null,
  fields: FieldResolver
): number {
  const fixed = Math.round(
    finiteIn(raw.width, 1, 10) ?? canvasConnectionDefaults.width
  )
  const name = fieldName(raw.widthField)
  const bound = name === null ? null : bind(name)
  const value = bound?.entry.value
  if (!bound || value === null || value === undefined) return fixed
  const config = fields.configFor(bound.entry.frame, bound.entry.field)
  const data = fieldRange(bound.entry.field)
  const min = config.min ?? data.min
  const max = config.max ?? data.max
  if (min === undefined || max === undefined) return fixed
  return scaledWidth(
    value,
    { min, max },
    {
      min: finiteIn(raw.minWidth, 1, 10) ?? undefined,
      max: finiteIn(raw.maxWidth, 1, 10) ?? undefined,
    }
  )
}

function lineOf(
  raw: unknown,
  index: number,
  placed: Map<string, Placed>,
  body: Size,
  fields: FieldResolver,
  bind: (name: string | null) => Bound | null
): CanvasLine | null {
  if (!isRecord(raw)) return null
  const from = typeof raw.from === "string" ? placed.get(raw.from) : undefined
  const to = typeof raw.to === "string" ? placed.get(raw.to) : undefined
  if (!from || !to) return null
  const vertices = verticesOf(raw.vertices, body)
  const start = edgePoint(
    from.box,
    from.parsed.kind,
    vertices[0] ?? center(to.box)
  )
  const end = edgePoint(
    to.box,
    to.parsed.kind,
    vertices[vertices.length - 1] ?? center(from.box)
  )
  const bare = vertices.length === 0
  if (bare && Math.hypot(end[0] - start[0], end[1] - start[1]) === 0) {
    return null
  }
  const bound = bind(typeof raw.field === "string" ? raw.field : null)
  return {
    key: `${index}:${from.parsed.id}:${to.parsed.id}`,
    x1: start[0],
    y1: start[1],
    x2: end[0],
    y2: end[1],
    points: [start, ...vertices, end].map(([x, y]) => ({ x, y })),
    path: oneOf(paths, raw.path) ?? canvasConnectionDefaults.path,
    lineStyle:
      oneOf(lineStyles, raw.lineStyle) ?? canvasConnectionDefaults.lineStyle,
    arrow: oneOf(arrows, raw.arrow) ?? canvasConnectionDefaults.arrow,
    color: cssColor(colorRef(raw.color), bound) ?? defaultLineColor,
    width: widthOf(raw, bind, fields),
  }
}

const liveRgb: RgbOf = (color) => cssColorToRgb(resolveColor(color))

function stateOf(parsed: Parsed, bound: Bound | null): MetricState | null {
  if (parsed.kind !== "metric" || !bound?.shown.color) return null
  if (![parsed.fill, parsed.stroke, parsed.textColor].includes("field")) {
    return null
  }
  if (!safeColor.test(bound.shown.color)) return null
  const { level, label } = bound.shown.threshold
  const onPlainBackground = parsed.fill === null && parsed.textColor === "field"
  return {
    level,
    label,
    color: bound.shown.color,
    labelColor: onPlainBackground ? foregroundColor : null,
  }
}

function textOn(
  fill: string | null,
  textColor: string | null,
  rgbOf: RgbOf
): string | null {
  if (textColor) return textColor
  if (!fill) return null
  const rgb = rgbOf(fill)
  return rgb ? readableText(rgb) : null
}

function rootColor(value: unknown): string | null {
  return typeof value === "string" && specColor.test(value)
    ? thresholdColor(value)
    : null
}

function lastText(
  frames: PreparedFrame[],
  fields: FieldResolver,
  name: string
): string | null {
  let found: string | null = null
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (field.type !== "string") continue
      const matches =
        field.name === name ||
        fields.seriesName(frame, field) === name ||
        fields.displayName(frame, field) === name
      if (!matches) continue
      for (const value of field.values) {
        if (typeof value === "string" && value !== "") found = value
      }
    }
  }
  return found
}

const angleFields = {
  droneTop: "yawField",
  droneFront: "rollField",
  droneSide: "pitchField",
} as const

export function canvasModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>,
  body: Size,
  rgbOf: RgbOf = liveRgb
): CanvasModel {
  const entries = entriesOf(frames, fields, display.reduce as Reducer)
  const shownCache = new Map<Entry, Bound>()
  const bind = (name: string | null): Bound | null => {
    const entry =
      name === null
        ? entries[0]
        : entries.find(
            (candidate) =>
              candidate.field.name === name ||
              candidate.seriesName === name ||
              candidate.displayName === name
          )
    if (!entry) return null
    const cached = shownCache.get(entry)
    if (cached) return cached
    const shown = displayValue(
      entry.value,
      fields.configFor(entry.frame, entry.field)
    )
    const color =
      shown.color && safeColor.test(shown.color) ? shown.color : entry.slotColor
    const bound = { entry, shown, color }
    shownCache.set(entry, bound)
    return bound
  }
  const bindNamed = (name: string | null): Bound | null =>
    name === null ? null : bind(name)
  const urlOf = (ref: ImageRef | null): string | null => {
    if (ref === null) return null
    return "url" in ref ? ref.url : lastText(frames, fields, ref.field)
  }
  const backgroundOf = (
    ref: ImageRef | null,
    size: ImageSize
  ): Background | null => {
    const url = urlOf(ref)
    return url === null ? null : { url, size }
  }

  const parsedElements = (
    Array.isArray(display.elements) ? display.elements : []
  ).flatMap((raw) => parseElement(raw) ?? [])
  const placed = new Map<string, Placed>(
    parsedElements.map((parsed) => [
      parsed.id,
      { parsed, box: elementBox(parsed.percent, parsed.placement, body) },
    ])
  )

  const elements = parsedElements.map((parsed, index): CanvasElement => {
    const bound = bind(parsed.field)
    const fill = cssColor(parsed.fill, bound)
    const textColor = cssColor(parsed.textColor, bound)
    const details: Detail[] = []
    if (bound && (parsed.field !== null || parsed.kind === "metric")) {
      details.push({ name: bound.entry.displayName, text: bound.shown.text })
    }
    const rate =
      parsed.kind === "windTurbine" ? bindNamed(parsed.rateField) : null
    if (rate) {
      details.push({ name: rate.entry.displayName, text: rate.shown.text })
    }
    const angleField =
      parsed.kind in angleFields
        ? parsed[angleFields[parsed.kind as keyof typeof angleFields]]
        : null
    return {
      key: `${index}:${parsed.id}`,
      id: parsed.id,
      kind: parsed.kind,
      box: elementBox(parsed.percent, parsed.placement, body),
      text: parsed.text,
      valueText:
        parsed.kind === "metric" ? (bound?.shown.text ?? noDataText) : null,
      fill,
      stroke: cssColor(parsed.stroke, bound),
      textColor: textOn(fill, textColor, rgbOf),
      fontSize: parsed.fontSize,
      align: parsed.align,
      valign: parsed.valign,
      rotation: parsed.rotation,
      borderWidth: parsed.borderWidth,
      radius: parsed.radius ?? canvasElementDefaults.radius,
      ownRadius: parsed.radius,
      background: backgroundOf(parsed.background, parsed.backgroundSize),
      icon: parsed.icon,
      image: urlOf(parsed.image),
      iconColor: cssColor(parsed.iconColor, bound),
      server: parsed.server,
      buttonStyle: parsed.buttonStyle,
      accent: cssColor(parsed.accentColor, bound),
      angle: bindNamed(angleField)?.shown.numeric ?? 0,
      details,
      state: stateOf(parsed, bound),
    }
  })

  const lines = (
    Array.isArray(display.connections) ? display.connections : []
  ).flatMap(
    (raw, index) => lineOf(raw, index, placed, body, fields, bind) ?? []
  )

  const root: CanvasRoot = {
    backgroundColor: rootColor(display.backgroundColor),
    borderColor: rootColor(display.borderColor),
    borderWidth: finiteIn(display.borderWidth, 0, 20) ?? 0,
    borderRadius: finiteIn(display.borderRadius, 0, 100) ?? 0,
    background: backgroundOf(
      imageRef(display.backgroundImage),
      oneOf(imageSizes, display.backgroundSize) ?? defaultImageSize
    ),
    panZoom: display.panZoom === true,
    infinitePan: display.infinitePan === true,
    tooltip: display.tooltip === "details",
  }
  return { root, elements, lines }
}
