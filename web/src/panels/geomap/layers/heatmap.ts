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

export function heatmapModel(
  rows: readonly HeatRow[],
  layer: Record<string, unknown>
): HeatmapModel {
  const weights = rows.map((row) => weightOf(row.weight))
  const largest = Math.max(0, ...weights)
  return {
    points: rows.map((row, index) => ({
      ...row,
      ...project(row.lon, row.lat),
      weight: largest === 0 ? 0 : weights[index] / largest,
    })),
    radius: isNumber(layer.radius) ? layer.radius : defaultRadius,
    blur: isNumber(layer.blur) ? layer.blur : defaultBlur,
  }
}
