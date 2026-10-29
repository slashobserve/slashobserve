import { thresholdColor } from "../thresholds"
import type { Cell } from "../timeseries/stack"
import {
  matchesName,
  type PreparedField,
  type PreparedFrame,
} from "../transforms"

export type CandleMode = "candles+volume" | "candles" | "volume"
export type CandleStyle = "candles" | "ohlcbars"
export type ColorStrategy = "open-close" | "close-close"
export type Direction = "up" | "down"
export type CandleRole = "open" | "high" | "low" | "close" | "volume"

export type CandleOptions = {
  mode: CandleMode
  candleStyle: CandleStyle
  colorStrategy: ColorStrategy
  upColor: string
  downColor: string
  legend: "bottom" | "right" | "hidden"
  tooltip: "single" | "all" | "hidden"
  names: Partial<Record<CandleRole, string>>
}

export type CandleColumn = { role: CandleRole; field: PreparedField }

export type CandleSelection =
  | { reason: string }
  | { reason: null; frame: PreparedFrame; columns: CandleColumn[] }

export const noCandlesReason =
  "A candlestick needs open, high, low and close fields."
export const noVolumeReason = "A candlestick needs a volume field."

export const volumeShare = 0.2

const priceRoles = ["open", "high", "low", "close"] as const
const maxBodyWidth = 64

export const roleLabels: Record<CandleRole, string> = {
  open: "Open",
  high: "High",
  low: "Low",
  close: "Close",
  volume: "Volume",
}

function textOf(value: unknown, fallback: string): string {
  return typeof value === "string" && value !== "" ? value : fallback
}

export function candleOptions(display: Record<string, unknown>): CandleOptions {
  const names: CandleOptions["names"] = {}
  for (const role of [...priceRoles, "volume"] as const) {
    const name = display[role]
    if (typeof name === "string" && name !== "") names[role] = name
  }
  return {
    mode:
      display.mode === "candles" || display.mode === "volume"
        ? display.mode
        : "candles+volume",
    candleStyle: display.candleStyle === "ohlcbars" ? "ohlcbars" : "candles",
    colorStrategy:
      display.colorStrategy === "close-close" ? "close-close" : "open-close",
    upColor: textOf(display.upColor, "ok"),
    downColor: textOf(display.downColor, "critical"),
    legend:
      display.legend === "right" || display.legend === "hidden"
        ? display.legend
        : "bottom",
    tooltip:
      display.tooltip === "all" || display.tooltip === "hidden"
        ? display.tooltip
        : "single",
    names,
  }
}

export function findCandleField(
  frame: PreparedFrame,
  role: CandleRole,
  explicit: string | undefined
): PreparedField | undefined {
  const numbers = frame.fields.filter((field) => field.type === "number")
  const named =
    explicit === undefined
      ? undefined
      : numbers.find((field) => matchesName(frame, field, explicit))
  return named ?? numbers.find((field) => field.name.toLowerCase() === role)
}

function columnsFor(
  frame: PreparedFrame,
  options: CandleOptions
): CandleColumn[] | null {
  const find = (role: CandleRole) =>
    findCandleField(frame, role, options.names[role])
  const prices = priceRoles.flatMap((role) => {
    const field = find(role)
    return field ? [{ role, field }] : []
  })
  const hasPrices = prices.length === priceRoles.length
  const volume = find("volume")
  const columns: CandleColumn[] = hasPrices ? prices : []
  if (options.mode !== "volume" && !hasPrices) return null
  if (options.mode !== "candles") {
    if (!volume) return options.mode === "volume" ? null : columns
    columns.push({ role: "volume", field: volume })
  }
  return columns
}

export function selectCandles(
  frames: readonly PreparedFrame[],
  options: CandleOptions
): CandleSelection {
  for (const frame of frames) {
    const time = frame.fields.find((field) => field.type === "time")
    const columns = time ? columnsFor(frame, options) : null
    if (time && columns) {
      return {
        reason: null,
        frame: { ...frame, fields: [time, ...columns.map((c) => c.field)] },
        columns,
      }
    }
  }
  return {
    reason: options.mode === "volume" ? noVolumeReason : noCandlesReason,
  }
}

function isNumber(value: Cell): value is number {
  return typeof value === "number" && Number.isFinite(value)
}

export function directions(
  open: readonly Cell[],
  close: readonly Cell[],
  strategy: ColorStrategy
): (Direction | null)[] {
  let previousClose: number | null = null
  return close.map((value, row) => {
    if (!isNumber(value)) return null
    const own = open[row]
    const reference = strategy === "open-close" ? own : (previousClose ?? own)
    previousClose = value
    if (!isNumber(reference)) return null
    return value >= reference ? "up" : "down"
  })
}

export function directionColors(
  options: Pick<CandleOptions, "upColor" | "downColor">
): Record<Direction, string> {
  return {
    up: thresholdColor(options.upColor),
    down: thresholdColor(options.downColor),
  }
}

export function rowColors(
  open: readonly Cell[],
  close: readonly Cell[],
  rows: number,
  options: Pick<CandleOptions, "colorStrategy" | "upColor" | "downColor">
): string[] {
  const colors = directionColors(options)
  const moves = directions(open, close, options.colorStrategy)
  return Array.from({ length: rows }, (_, row) => colors[moves[row] ?? "up"])
}

export function minSpacing(positions: readonly number[]): number {
  let spacing = Infinity
  for (let index = 1; index < positions.length; index++) {
    const gap = positions[index] - positions[index - 1]
    if (gap > 0 && gap < spacing) spacing = gap
  }
  return spacing
}

export function bodyWidth(spacing: number, ratio: number): number {
  return Math.max(ratio, Math.min(spacing * 0.7, maxBodyWidth * ratio))
}

export function volumeHeight(
  value: Cell,
  maxVolume: number,
  plotHeight: number
): number {
  if (!isNumber(value) || value <= 0 || maxVolume <= 0) return 0
  return (value / maxVolume) * plotHeight * volumeShare
}
