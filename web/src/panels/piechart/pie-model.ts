import { decimalsByUnit } from "@/lib/units"
import type { Reducer } from "@/spec"

import { styleForSlot } from "../colors"
import {
  displayValue,
  type FieldConfig,
  type FieldResolver,
} from "../field-config"
import { seriesKey, uniqueKey } from "../frames"
import { reduce } from "../reducers"
import type { PreparedFrame } from "../transforms"

export type PieType = "pie" | "donut"
export type PieLegend = "right" | "bottom" | "hidden"

export type Slice = {
  key: string
  name: string
  value: number
  text: string
  percent: number
  color: string
  startAngle: number
  endAngle: number
}

export type LeftOut = {
  key: string
  name: string
  text: string
  color: string
  value: number | null
}

export type PieModel = { total: number; slices: Slice[]; left: LeftOut[] }

export const donutRatio = 0.6

const fullTurn = Math.PI * 2

export function reducerOf(display: Record<string, unknown>): Reducer {
  return display.reduce as Reducer
}

export function pieTypeOf(display: Record<string, unknown>): PieType {
  return display.pieType === "donut" ? "donut" : "pie"
}

export function legendOf(display: Record<string, unknown>): PieLegend {
  if (display.legend === "bottom") return "bottom"
  if (display.legend === "hidden") return "hidden"
  return "right"
}

type Entry = {
  key: string
  name: string
  config: FieldConfig
  value: number | null
  color: string
}

function entriesOf(
  frames: PreparedFrame[],
  fields: FieldResolver,
  reducer: Reducer
): Omit<Entry, "color">[] {
  const seen = new Map<string, number>()
  const entries: Omit<Entry, "color">[] = []
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (field.type !== "number") continue
      const config = fields.configFor(frame, field)
      if (config.hidden) continue
      entries.push({
        key: uniqueKey(seen, seriesKey(frame, field.name)),
        name: fields.displayName(frame, field),
        config,
        value: reduce(field.values, reducer),
      })
    }
  }
  return entries
}

function isDrawn(value: number | null): value is number {
  return value !== null && value > 0
}

export function pieModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>
): PieModel {
  const entries = entriesOf(frames, fields, reducerOf(display))
  const decimals = decimalsByUnit(
    entries.map(({ config, value }) => ({
      unit: config.unit,
      decimals: config.decimals,
      value,
    }))
  )
  const total = entries.reduce(
    (sum, { value }) => (isDrawn(value) ? sum + value : sum),
    0
  )
  const slices: Slice[] = []
  const left: LeftOut[] = []
  let angle = 0
  entries.forEach(({ key, name, config, value }, index) => {
    const shown = displayValue(value, config, {
      decimals: decimals.get(config.unit),
    })
    const color = shown.color ?? styleForSlot(index).color
    if (!isDrawn(value)) {
      left.push({ key, name, text: shown.text, color, value })
      return
    }
    const sweep = (value / total) * fullTurn
    slices.push({
      key,
      name,
      value,
      text: shown.text,
      percent: (value / total) * 100,
      color,
      startAngle: angle,
      endAngle: angle + sweep,
    })
    angle += sweep
  })
  return { total, slices, left }
}

export function percentText(percent: number): string {
  return `${percent.toFixed(1)}%`
}

export function sliceLabel(slice: Slice): string {
  return `${slice.name} ${slice.text} (${percentText(slice.percent)})`
}

export function summaryLabel(slices: Slice[]): string {
  return slices.slice(0, 5).map(sliceLabel).join(", ")
}

function round(value: number): number {
  return Math.round(value * 1000) / 1000
}

function point(cx: number, cy: number, r: number, angle: number): string {
  return `${round(cx + r * Math.sin(angle))} ${round(cy - r * Math.cos(angle))}`
}

function circle(cx: number, cy: number, r: number, clockwise: boolean) {
  const sweep = clockwise ? 1 : 0
  return (
    `M ${point(cx, cy, r, 0)} ` +
    `A ${r} ${r} 0 1 ${sweep} ${point(cx, cy, r, Math.PI)} ` +
    `A ${r} ${r} 0 1 ${sweep} ${point(cx, cy, r, 0)} Z`
  )
}

export function slicePath(
  cx: number,
  cy: number,
  r: number,
  inner: number,
  start: number,
  end: number
): string {
  if (end - start >= fullTurn - 1e-9) {
    const ring = inner > 0 ? ` ${circle(cx, cy, inner, false)}` : ""
    return `${circle(cx, cy, r, true)}${ring}`
  }
  const large = end - start > Math.PI ? 1 : 0
  const outer = `M ${point(cx, cy, r, start)} A ${r} ${r} 0 ${large} 1 ${point(cx, cy, r, end)}`
  if (inner <= 0) return `${outer} L ${cx} ${cy} Z`
  return `${outer} L ${point(cx, cy, inner, end)} A ${inner} ${inner} 0 ${large} 0 ${point(cx, cy, inner, start)} Z`
}
