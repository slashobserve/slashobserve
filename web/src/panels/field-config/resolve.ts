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
  width?: number
  minWidth?: number
  align: "auto" | "left" | "center" | "right"
  filterable: boolean
  inspect: boolean
  hideHeader: boolean
  valueTransform?: "negativeY" | "constant"
  hideInLegend: boolean
  hideInTooltip: boolean
  stackGroup: string
  fillBelowTo?: string
}

export type FieldResolver = {
  configFor(frame: PreparedFrame, field: PreparedField): FieldConfig
  isSet(
    frame: PreparedFrame,
    field: PreparedField,
    key: keyof FieldConfig
  ): boolean
  seriesName(frame: PreparedFrame, field: PreparedField): string
  displayName(frame: PreparedFrame, field: PreparedField): string
}

const fieldPropKeys = [
  "unit",
  "decimals",
  "min",
  "max",
  "thresholds",
  "thresholdMode",
  "color",
  "colorMode",
  "colorScheme",
  "colorBy",
  "displayName",
  "noValue",
  "mappings",
  "links",
  "style",
  "fill",
  "lineStyle",
  "lineInterpolation",
  "lineWidth",
  "points",
  "pointSize",
  "spanNulls",
  "stack",
  "axis",
  "hidden",
  "axisLabel",
  "axisSoftMin",
  "axisSoftMax",
  "scale",
  "logBase",
  "linearThreshold",
  "axisCenteredZero",
  "axisWidth",
  "axisGrid",
  "axisBorder",
  "axisColor",
  "cell",
  "width",
  "minWidth",
  "align",
  "filterable",
  "inspect",
  "hideHeader",
  "valueTransform",
  "hideInLegend",
  "hideInTooltip",
  "stackGroup",
  "fillBelowTo",
] as const satisfies readonly (keyof FieldProps)[]

export function effectiveColorMode(
  config: Pick<FieldConfig, "colorMode" | "color">
): ColorMode {
  return config.colorMode ?? (config.color ? "fixed" : "palette")
}

type Resolved = {
  seriesName: string
  displayName: string
  config: FieldConfig
  given: ReadonlySet<keyof FieldConfig>
}

function withProps(config: FieldConfig, props: FieldProps): FieldConfig {
  const given = fieldPropKeys.flatMap((key) =>
    props[key] === undefined ? [] : [[key, props[key]] as const]
  )
  return { ...config, ...Object.fromEntries(given) }
}

function givenKeys(props: FieldProps): (keyof FieldConfig)[] {
  return fieldPropKeys.filter((key) => props[key] !== undefined)
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
  const baseGiven = givenKeys(panel)
  const overrides = (panel.overrides ?? []).map((override) => ({
    matches: compileMatch(override.match),
    props: override,
  }))
  const cache = new WeakMap<PreparedFrame, WeakMap<PreparedField, Resolved>>()

  function resolve(frame: PreparedFrame, field: PreparedField): Resolved {
    const seriesName = preparedSeriesName(frame, field)
    let config = field.config ? withProps(base, field.config) : base
    const given = new Set(baseGiven)
    if (field.config) givenKeys(field.config).forEach((key) => given.add(key))
    for (const { matches, props } of overrides) {
      if (matches({ frame, field, seriesName })) {
        config = withProps(config, props)
        givenKeys(props).forEach((key) => given.add(key))
      }
    }
    return {
      seriesName,
      displayName: displayNameOf(config.displayName, frame, field, seriesName),
      config,
      given,
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
    isSet: (frame, field, key) => resolved(frame, field).given.has(key),
    seriesName: (frame, field) => resolved(frame, field).seriesName,
    displayName: (frame, field) => resolved(frame, field).displayName,
  }
}
