import { isNumber } from "../../reducers"
import { project } from "../projection"

export type HeatRow = {
  frameIndex: number
  row: number
  lat: number
  lon: number
  weight: number
}

type HeatPoint = HeatRow & { x: number; y: number }

export type HeatmapModel = {
  points: HeatPoint[]
  radius: number
  blur: number
}

const defaultRadius = 15
const defaultBlur = 15

function weightOf(value: number): number {
  return Number.isFinite(value) && value > 0 ? value : 0
}

function unit(value: number): number {
  return Math.max(0, Math.min(1, value))
}

function scaledWeights(
  rows: readonly HeatRow[],
  layer: Record<string, unknown>
): number[] {
  const weights = rows.map((row) => weightOf(row.weight))
  const largest = Math.max(0, ...weights)
  const low = isNumber(layer.weightMin) ? layer.weightMin : 0
  const high = isNumber(layer.weightMax) ? layer.weightMax : 1
  return weights.map((weight) =>
    unit(low + (largest === 0 ? 0 : weight / largest) * (high - low))
  )
}

export function heatmapModel(
  rows: readonly HeatRow[],
  layer: Record<string, unknown>,
  weighted: boolean
): HeatmapModel {
  const fixed = unit(isNumber(layer.weight) ? layer.weight : 1)
  const weights = weighted ? scaledWeights(rows, layer) : rows.map(() => fixed)
  return {
    points: rows.map((row, index) => ({
      ...row,
      ...project(row.lon, row.lat),
      weight: weights[index],
    })),
    radius: isNumber(layer.radius) ? layer.radius : defaultRadius,
    blur: isNumber(layer.blur) ? layer.blur : defaultBlur,
  }
}
