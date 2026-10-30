import {
  seriesName as preparedSeriesName,
  type PreparedField,
  type PreparedFrame,
} from "@/panels/transforms"
import {
  fieldPropDefaults,
  type DataPanel,
  type FieldProps,
  type Link,
  type Mapping,
  type Threshold,
} from "@/spec"

import type { ColorScheme } from "../color-scheme"
import { interpolateDisplayName } from "./display-name"
import { compileMatch } from "./match"

type ColorMode = NonNullable<FieldProps["colorMode"]>

export type FieldConfig = {
  unit?: string
  decimals?: number
  min?: number
  max?: number
  thresholds?: Threshold[]
  thresholdMode: "absolute" | "percentage"
  color?: string
  colorMode?: ColorMode
  colorScheme?: ColorScheme
  colorBy: "last" | "min" | "max"
  displayName?: string
  noValue?: string
  mappings: Mapping[]
  links: Link[]
  style: "line" | "bars" | "points"
  fill: number
  lineStyle: "solid" | "dashed" | "dotted"
  lineInterpolation: "linear" | "smooth" | "stepBefore" | "stepAfter"
  lineWidth: number
  points: "auto" | "always" | "never"
  pointSize: number
  spanNulls: boolean | number
  stack: "none" | "normal" | "percent"
  axis: "left" | "right" | "hidden"
  hidden: boolean
  axisLabel?: string
  axisSoftMin?: number
  axisSoftMax?: number
  scale: "linear" | "log" | "symlog"
  logBase: 2 | 10
  linearThreshold?: number
  axisCenteredZero: boolean
  axisWidth?: number
  axisGrid?: boolean
  axisBorder: boolean
  axisColor: "text" | "series"
  cell?: FieldProps["cell"]
  valueTransform?: "negativeY" | "constant"
  hideInLegend: boolean
  hideInTooltip: boolean
  stackGroup: string
  fillBelowTo?: string
}

export type FieldResolver = {
  configFor(frame: PreparedFrame, field: PreparedField): FieldConfig
  seriesName(frame: PreparedFrame, field: PreparedField): string
  displayName(frame: PreparedFrame, field: PreparedField): string
}

const fieldPropKeyFlags: Record<keyof FieldProps, true> = {
  unit: true,
  decimals: true,
  min: true,
  max: true,
  thresholds: true,
  thresholdMode: true,
  color: true,
  colorMode: true,
  colorScheme: true,
  colorBy: true,
  displayName: true,
  noValue: true,
  mappings: true,
  links: true,
  style: true,
  fill: true,
  lineStyle: true,
  lineInterpolation: true,
  lineWidth: true,
  points: true,
  pointSize: true,
  spanNulls: true,
  stack: true,
  axis: true,
  hidden: true,
  axisLabel: true,
  axisSoftMin: true,
  axisSoftMax: true,
  scale: true,
  logBase: true,
  linearThreshold: true,
  axisCenteredZero: true,
  axisWidth: true,
  axisGrid: true,
  axisBorder: true,
  axisColor: true,
  cell: true,
  valueTransform: true,
  hideInLegend: true,
  hideInTooltip: true,
  stackGroup: true,
  fillBelowTo: true,
}

const fieldPropKeys = Object.keys(fieldPropKeyFlags) as (keyof FieldProps)[]

export function effectiveColorMode(
  config: Pick<FieldConfig, "colorMode" | "color">
): ColorMode {
  return config.colorMode ?? (config.color ? "fixed" : "palette")
}

type Resolved = { seriesName: string; displayName: string; config: FieldConfig }

function withProps(config: FieldConfig, props: FieldProps): FieldConfig {
  const given = fieldPropKeys.flatMap((key) =>
    props[key] === undefined ? [] : [[key, props[key]] as const]
  )
  return { ...config, ...Object.fromEntries(given) }
}

function displayNameOf(
  template: string | undefined,
  frame: PreparedFrame,
  field: PreparedField,
  seriesName: string
): string {
  return template === undefined
    ? seriesName
    : interpolateDisplayName(template, frame, field.name, seriesName)
}

export function createFieldResolver(panel: DataPanel): FieldResolver {
  const base = withProps(
    {
      ...fieldPropDefaults,
      mappings: [],
      links: [],
    } as FieldConfig,
    panel
  )
  const overrides = (panel.overrides ?? []).map((override) => ({
    matches: compileMatch(override.match),
    props: override,
  }))
  const cache = new WeakMap<PreparedFrame, WeakMap<PreparedField, Resolved>>()

  function resolve(frame: PreparedFrame, field: PreparedField): Resolved {
    const seriesName = preparedSeriesName(frame, field)
    let config = field.config ? withProps(base, field.config) : base
    for (const { matches, props } of overrides) {
      if (matches({ frame, field, seriesName })) {
        config = withProps(config, props)
      }
    }
    return {
      seriesName,
      displayName: displayNameOf(config.displayName, frame, field, seriesName),
      config,
    }
  }

  function resolved(frame: PreparedFrame, field: PreparedField): Resolved {
    let byField = cache.get(frame)
    if (!byField) {
      byField = new WeakMap()
      cache.set(frame, byField)
    }
    let entry = byField.get(field)
    if (!entry) {
      entry = resolve(frame, field)
      byField.set(field, entry)
    }
    return entry
  }

  return {
    configFor: (frame, field) => resolved(frame, field).config,
    seriesName: (frame, field) => resolved(frame, field).seriesName,
    displayName: (frame, field) => resolved(frame, field).displayName,
  }
}
