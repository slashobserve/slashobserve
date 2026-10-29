import {
  canvasConnectionDefaults,
  canvasElementDefaults,
  type Reducer,
} from "@/spec"

import { styleForSlot, resolveColor } from "../colors"
import { cssColorToRgb, readableText, type Rgb } from "../contrast"
import {
  displayValue,
  type DisplayValue,
  type FieldResolver,
} from "../field-config"
import { reduce } from "../reducers"
import { thresholdColor, type ThresholdLevel } from "../thresholds"
import type { PreparedField, PreparedFrame } from "../transforms"

type CanvasKind = "text" | "metric" | "rectangle" | "ellipse"
export type CanvasAlign = "left" | "center" | "right"
export type RgbOf = (color: string) => Rgb | null

type Box = { left: number; top: number; width: number; height: number }

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
  state: MetricState | null
}

export type CanvasLine = {
  key: string
  x1: number
  y1: number
  x2: number
  y2: number
  color: string
  width: number
}

export type CanvasModel = { elements: CanvasElement[]; lines: CanvasLine[] }

const noDataText = "No data"
export const defaultLineColor = "var(--muted-foreground)"
export const foregroundColor = "var(--foreground)"

const kinds: readonly string[] = ["text", "metric", "rectangle", "ellipse"]
const aligns: readonly string[] = ["left", "center", "right"]
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

type Bound = { shown: DisplayValue; color: string }

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

type Parsed = {
  id: string
  kind: CanvasKind
  box: Box
  text: string | null
  field: string | null
  fill: ColorRef
  stroke: ColorRef
  textColor: ColorRef
  fontSize: number
  align: CanvasAlign
}

function parseElement(raw: unknown): Parsed | null {
  if (!isRecord(raw)) return null
  const { id, kind } = raw
  if (typeof id !== "string" || typeof kind !== "string") return null
  if (!kinds.includes(kind)) return null
  const place = box(raw)
  if (!place) return null
  const align =
    typeof raw.align === "string" && aligns.includes(raw.align)
      ? raw.align
      : canvasElementDefaults.align
  return {
    id,
    kind: kind as CanvasKind,
    box: place,
    text: typeof raw.text === "string" && raw.text !== "" ? raw.text : null,
    field: typeof raw.field === "string" ? raw.field : null,
    fill: colorRef(raw.fill),
    stroke: colorRef(raw.stroke),
    textColor: colorRef(raw.textColor),
    fontSize: finiteIn(raw.fontSize, 8, 72) ?? canvasElementDefaults.fontSize,
    align: align as CanvasAlign,
  }
}

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

function lineOf(
  raw: unknown,
  index: number,
  placed: Map<string, Parsed>,
  bind: (name: string | null) => Bound | null
): CanvasLine | null {
  if (!isRecord(raw)) return null
  const from = typeof raw.from === "string" ? placed.get(raw.from) : undefined
  const to = typeof raw.to === "string" ? placed.get(raw.to) : undefined
  if (!from || !to) return null
  const [fromCenter, toCenter] = [center(from.box), center(to.box)]
  const start = edgePoint(from.box, from.kind, toCenter)
  const end = edgePoint(to.box, to.kind, fromCenter)
  if (Math.hypot(end[0] - start[0], end[1] - start[1]) === 0) return null
  const bound = bind(typeof raw.field === "string" ? raw.field : null)
  return {
    key: `${index}:${from.id}:${to.id}`,
    x1: start[0],
    y1: start[1],
    x2: end[0],
    y2: end[1],
    color: cssColor(colorRef(raw.color), bound) ?? defaultLineColor,
    width: Math.round(
      finiteIn(raw.width, 1, 10) ?? canvasConnectionDefaults.width
    ),
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

export function canvasModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>,
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
    const bound = { shown, color }
    shownCache.set(entry, bound)
    return bound
  }

  const parsedElements = (
    Array.isArray(display.elements) ? display.elements : []
  ).flatMap((raw) => parseElement(raw) ?? [])
  const placed = new Map(parsedElements.map((element) => [element.id, element]))

  const elements = parsedElements.map((element, index): CanvasElement => {
    const bound = bind(element.field)
    const fill = cssColor(element.fill, bound)
    const textColor = cssColor(element.textColor, bound)
    return {
      key: `${index}:${element.id}`,
      id: element.id,
      kind: element.kind,
      box: element.box,
      text: element.text,
      valueText:
        element.kind === "metric" ? (bound?.shown.text ?? noDataText) : null,
      fill,
      stroke: cssColor(element.stroke, bound),
      textColor: textOn(fill, textColor, rgbOf),
      fontSize: element.fontSize,
      align: element.align,
      state: stateOf(element, bound),
    }
  })

  const lines = (
    Array.isArray(display.connections) ? display.connections : []
  ).flatMap((raw, index) => lineOf(raw, index, placed, bind) ?? [])
  return { elements, lines }
}
