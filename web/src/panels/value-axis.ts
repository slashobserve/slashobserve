import type { FieldConfig } from "./field-config"
import {
  centeredRange,
  logRange,
  softRange,
  type LogBase,
  type ScaleKind,
} from "./timeseries/axis-scale"

export type AxisSide = "start" | "end" | "hidden"

export type ValueAxisSettings = {
  side: AxisSide
  label?: string
  width?: number
  grid: boolean
  border: boolean
  seriesColored: boolean
  scale: ScaleKind
  logBase: LogBase
  linearThreshold: number
  softMin?: number
  softMax?: number
  centeredZero: boolean
}

export type ValueScale = {
  kind: ScaleKind
  logBase: LogBase
  linearThreshold: number
}

export const linearScale: ValueScale = {
  kind: "linear",
  logBase: 10,
  linearThreshold: 1,
}

const sideOf: Record<FieldConfig["axis"], AxisSide> = {
  left: "start",
  right: "end",
  hidden: "hidden",
}

export function valueAxisSettings(
  config: Pick<
    FieldConfig,
    | "axis"
    | "axisLabel"
    | "axisWidth"
    | "axisGrid"
    | "axisBorder"
    | "axisColor"
    | "scale"
    | "logBase"
    | "linearThreshold"
    | "axisSoftMin"
    | "axisSoftMax"
    | "axisCenteredZero"
  >
): ValueAxisSettings {
  return {
    side: sideOf[config.axis],
    label: config.axisLabel,
    width: config.axisWidth,
    grid: config.axisGrid ?? true,
    border: config.axisBorder,
    seriesColored: config.axisColor === "series",
    scale: config.scale,
    logBase: config.logBase,
    linearThreshold: config.linearThreshold ?? 1,
    softMin: config.axisSoftMin,
    softMax: config.axisSoftMax,
    centeredZero: config.axisCenteredZero,
  }
}

export function scaleOf(settings: ValueAxisSettings): ValueScale {
  return {
    kind: settings.scale,
    logBase: settings.logBase,
    linearThreshold: settings.linearThreshold,
  }
}

export function valueRange(
  auto: [number, number],
  positiveMin: number,
  settings: ValueAxisSettings
): [number, number] {
  if (settings.scale === "log") {
    const [low, high] = softRange(
      [positiveMin, auto[1]],
      settings.softMin,
      settings.softMax
    )
    return logRange(low, high, settings.logBase)
  }
  const soft = softRange(auto, settings.softMin, settings.softMax)
  return settings.centeredZero ? centeredRange(soft) : soft
}

function asinhPosition(value: number, threshold: number): number {
  return Math.asinh(value / threshold)
}

export function scalePosition(
  value: number,
  low: number,
  high: number,
  scale: ValueScale
): number {
  if (scale.kind === "log") {
    if (value <= 0 || low <= 0 || high <= low) return 0
    return Math.log(value / low) / Math.log(high / low)
  }
  const [from, to, at] =
    scale.kind === "symlog"
      ? [
          asinhPosition(low, scale.linearThreshold),
          asinhPosition(high, scale.linearThreshold),
          asinhPosition(value, scale.linearThreshold),
        ]
      : [low, high, value]
  return to === from ? 0 : (at - from) / (to - from)
}

function thinned<T>(items: readonly T[], maxCount: number): T[] {
  const limit = Math.max(2, Math.floor(maxCount))
  const stride = Math.max(1, Math.ceil(items.length / limit))
  return items.filter((_, index) => index % stride === 0)
}

export function logTicks(
  low: number,
  high: number,
  base: LogBase,
  maxTicks: number
): number[] {
  if (!(low > 0) || !(high >= low)) return []
  const exponentOf = base === 2 ? Math.log2 : Math.log10
  const first = Math.round(exponentOf(low))
  const last = Math.round(exponentOf(high))
  const powers = Array.from(
    { length: Math.max(0, last - first) + 1 },
    (_, i) => (first + i < 0 ? 1 / base ** -(first + i) : base ** (first + i))
  )
  return thinned(powers, maxTicks)
}

export function symlogTicks(
  low: number,
  high: number,
  threshold: number,
  maxTicks: number
): number[] {
  const reach = Math.max(Math.abs(low), Math.abs(high))
  const magnitudes: number[] = []
  for (let m = threshold; m <= reach && magnitudes.length < 64; m *= 10) {
    magnitudes.push(Number(m.toPrecision(12)))
  }
  const kept = thinned(magnitudes, Math.floor((maxTicks - 1) / 2))
  return [...kept.map((m) => -m).reverse(), 0, ...kept].filter(
    (tick) => tick >= low && tick <= high
  )
}
