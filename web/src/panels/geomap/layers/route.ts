import { isNumber } from "../../reducers"
import { styleForSlot } from "../../colors"
import type { Tip } from "../geo-model"
import { project } from "../projection"

export type Arrow = "none" | "forward" | "reverse" | "both"

export type RouteRow = {
  frameIndex: number
  row: number
  lat: number
  lon: number
  time: number | null
  tip: Tip
}

type RoutePoint = Omit<RouteRow, "time"> & {
  key: string
  x: number
  y: number
}

type RouteLine = {
  key: string
  frameIndex: number
  color: string
  points: RoutePoint[]
}

export type RouteModel = {
  lines: RouteLine[]
  lineWidth: number
  arrow: Arrow
}

const defaultLineWidth = 2
const arrows: readonly Arrow[] = ["none", "forward", "reverse", "both"]

export function arrowOf(value: unknown): Arrow {
  return arrows.find((arrow) => arrow === value) ?? "none"
}

function inTimeOrder(rows: RouteRow[]): RouteRow[] {
  if (rows.some((row) => row.time === null)) return rows
  return rows
    .map((row, index) => ({ row, index }))
    .sort(
      (a, b) =>
        (a.row.time as number) - (b.row.time as number) || a.index - b.index
    )
    .map(({ row }) => row)
}

export function routeModel(
  rows: readonly RouteRow[],
  layer: Record<string, unknown>,
  fixed: string | undefined
): RouteModel {
  const frames = new Map<number, RouteRow[]>()
  for (const row of rows) {
    const own = frames.get(row.frameIndex)
    if (own) own.push(row)
    else frames.set(row.frameIndex, [row])
  }
  const lines = [...frames].map(([frameIndex, own]): RouteLine => {
    const points = inTimeOrder(own).map((point): RoutePoint => ({
      key: `${point.frameIndex}:${point.row}`,
      frameIndex: point.frameIndex,
      row: point.row,
      lat: point.lat,
      lon: point.lon,
      tip: point.tip,
      ...project(point.lon, point.lat),
    }))
    return {
      key: String(frameIndex),
      frameIndex,
      color: fixed ?? styleForSlot(frameIndex).color,
      points,
    }
  })
  return {
    lines,
    lineWidth: isNumber(layer.lineWidth) ? layer.lineWidth : defaultLineWidth,
    arrow: arrowOf(layer.arrow),
  }
}
