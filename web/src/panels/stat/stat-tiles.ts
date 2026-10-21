import { commonDecimals } from "@/lib/units"
import type { WireField } from "@/panels/frames"
import {
  displayValue,
  fieldRange,
  type FieldResolver,
} from "@/panels/field-config"
import { reduce, reducerLabels } from "@/panels/reducers"
import type { ThresholdState } from "@/panels/thresholds"
import type { PreparedFrame } from "@/panels/transforms"
import type { Link, Reducer } from "@/spec"

export type ColorMode = "value" | "background" | "none"
export type TextMode = "value" | "name" | "valueAndName" | "none"
export type Orientation = "auto" | "horizontal" | "vertical"

export type StatDisplay = {
  reduce: Reducer
  colorMode: ColorMode
  graph: boolean
  orientation: Orientation
  textMode: TextMode | undefined
}

export type StatTile = {
  key: string
  name: string
  seriesName: string
  fieldName: string
  labels: Record<string, string> | undefined
  value: number | null
  text: string
  color: string | null
  threshold: ThresholdState
  points: [number, number][]
  links: Link[]
}

function oneOf<T extends string>(
  value: unknown,
  allowed: readonly T[]
): T | undefined {
  return allowed.find((candidate) => candidate === value)
}

const reducers = Object.keys(reducerLabels) as Reducer[]

export function statDisplay(display: Record<string, unknown>): StatDisplay {
  return {
    reduce: oneOf(display.reduce, reducers) ?? "lastNotNull",
    colorMode:
      oneOf(display.colorMode, ["value", "background", "none"]) ?? "value",
    graph: display.graph !== false,
    orientation:
      oneOf(display.orientation, ["auto", "horizontal", "vertical"]) ?? "auto",
    textMode: oneOf(display.textMode, [
      "value",
      "name",
      "valueAndName",
      "none",
    ]),
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
  display: Pick<StatDisplay, "reduce">,
  unit: string | undefined
): StatTile[] {
  const entries = frames.flatMap((frame) =>
    frame.fields
      .filter((field) => field.type === "number")
      .flatMap((field) => {
        const config = fields.configFor(frame, field)
        return config.hidden
          ? []
          : [
              {
                frame,
                field,
                config,
                value: reduce(field.values, display.reduce),
              },
            ]
      })
  )
  const decimals = commonDecimals(
    entries.map((entry) => entry.value),
    unit
  )
  const seen = new Map<string, number>()
  return entries.map(({ frame, field, config, value }) => {
    const seriesName = fields.seriesName(frame, field)
    const range = fieldRange(field)
    const shown = displayValue(value, config, {
      decimals,
      fieldMin: range.min,
      fieldMax: range.max,
    })
    const occurrence = seen.get(seriesName) ?? 0
    seen.set(seriesName, occurrence + 1)
    return {
      key: `${seriesName}#${occurrence}`,
      name: fields.displayName(frame, field),
      seriesName,
      fieldName: field.name,
      labels: frame.labels,
      value,
      text: shown.text,
      color: shown.color,
      threshold: shown.threshold,
      points: sparkline(frame, field),
      links: config.links,
    }
  })
}
