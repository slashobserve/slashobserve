import { commonDecimals, formatValueParts } from "@/lib/units"
import type { WireField } from "@/panels/frames"
import {
  displayValue,
  fieldRange,
  type DisplayValue,
  type FieldResolver,
} from "@/panels/field-config"
import { reduceItems, reduceOptionsOf } from "@/panels/reduce-items"
import type { ThresholdState } from "@/panels/thresholds"
import type { PreparedFrame } from "@/panels/transforms"
import type { Link, Reducer } from "@/spec"

type ColorMode = "value" | "background" | "none"
type TextMode = "value" | "name" | "valueAndName" | "none"
type Orientation = "auto" | "horizontal" | "vertical"
type PercentChangeColor = "standard" | "inverted" | "value"

export type StatDisplay = {
  reduce: Reducer
  colorMode: ColorMode
  graph: boolean
  orientation: Orientation
  textMode?: TextMode
  allValues?: boolean
  limit?: number
  fields?: string
  justify?: "auto" | "center"
  titleSize?: number
  valueSize?: number
  percentChange?: boolean
  percentChangeColor?: PercentChangeColor
  wideLayout?: boolean
}

export type ChangeDirection = "up" | "down" | "flat"
export type ChangeTone = "ok" | "critical" | "value" | "none"

export type PercentChange = {
  direction: ChangeDirection
  text: string
  word: "up" | "down" | "unchanged"
}

export type StatTile = {
  key: string
  name: string
  seriesName: string
  fieldName: string
  labels: Record<string, string> | undefined
  value: number | null
  text: string
  amount: string
  unit: string
  color: string | null
  threshold: ThresholdState
  points: [number, number][]
  change: PercentChange | null
  links: Link[]
}

export function percentChange(
  points: readonly [number, number][]
): PercentChange | null {
  if (points.length < 2) return null
  const first = points[0][1]
  const last = points[points.length - 1][1]
  if (first === 0) return null
  const percent = ((last - first) / Math.abs(first)) * 100
  const magnitude = Math.abs(percent)
  const rounded = Number(magnitude.toFixed(magnitude < 10 ? 1 : 0))
  if (rounded === 0) {
    return { direction: "flat", text: "0%", word: "unchanged" }
  }
  const direction = percent > 0 ? "up" : "down"
  return { direction, text: `${rounded}%`, word: direction }
}

export function changeTone(
  direction: ChangeDirection,
  mode: PercentChangeColor
): ChangeTone {
  if (direction === "flat") return "none"
  if (mode === "value") return "value"
  const rising = direction === "up"
  return rising === (mode === "standard") ? "ok" : "critical"
}

function amountAndUnit(
  shown: DisplayValue,
  unit: string | undefined,
  decimals: number | undefined
) {
  if (shown.mapped || shown.numeric === null) {
    return { amount: shown.text, unit: "" }
  }
  const parts = formatValueParts(shown.numeric, unit, decimals)
  return {
    amount: `${parts.prefix}${parts.amount}`,
    unit: parts.suffix.trim(),
  }
}

function sparkline(frame: PreparedFrame, field: WireField): [number, number][] {
  const time = frame.fields.find((candidate) => candidate.type === "time")
  if (!time) return []
  const points: [number, number][] = []
  field.values.forEach((value, index) => {
    const at = time.values[index]
    if (
      typeof at === "number" &&
      typeof value === "number" &&
      Number.isFinite(value)
    ) {
      points.push([at, value])
    }
  })
  return points
}

export function statTiles(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Partial<StatDisplay>,
  unit: string | undefined
): StatTile[] {
  const entries = reduceItems(frames, fields, reduceOptionsOf("stat", display))
  const decimals = commonDecimals(
    entries.map((entry) => entry.value),
    unit
  )
  return entries.map(
    ({ key, label, frame, field, config, value, raw, row }) => {
      const seriesName = fields.seriesName(frame, field)
      const range = fieldRange(field)
      const points = row === undefined ? sparkline(frame, field) : []
      const shown = displayValue(raw, config, {
        decimals,
        fieldMin: range.min,
        fieldMax: range.max,
      })
      return {
        key,
        name: label,
        seriesName,
        fieldName: field.name,
        labels: frame.labels,
        value,
        text: shown.text,
        ...amountAndUnit(shown, config.unit, config.decimals ?? decimals),
        color: shown.color,
        threshold: shown.threshold,
        points,
        change: percentChange(points),
        links: config.links,
      }
    }
  )
}
