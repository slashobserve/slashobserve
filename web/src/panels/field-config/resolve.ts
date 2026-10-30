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

import { interpolateDisplayName } from "./display-name"
import { compileMatch } from "./match"

export type FieldConfig = {
  unit?: string
  decimals?: number
  min?: number
  max?: number
  thresholds?: Threshold[]
  thresholdMode: "absolute" | "percentage"
  color?: string
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
}

export type FieldResolver = {
  configFor(frame: PreparedFrame, field: PreparedField): FieldConfig
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
] as const satisfies readonly (keyof FieldProps)[]

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
