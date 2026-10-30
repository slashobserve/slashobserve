import { isNumber } from "../../reducers"
import { project, type Point } from "../projection"
import { arrowOf, type Arrow } from "./route"

export type NetworkNode = { id: string; lat: number; lon: number }

export type EdgeRow = {
  frameIndex: number
  row: number
  source: unknown
  target: unknown
}

type NetworkEdge = { key: string; from: Point; to: Point }

export type NetworkModel = {
  edges: NetworkEdge[]
  unlocated: number
  truncated: number
  color: string | null
  lineWidth: number
  arrow: Arrow
}

export const maxEdges = 1_000

const defaultLineWidth = 1

export function idText(value: unknown): string | null {
  if (typeof value === "string") return value === "" ? null : value
  return isNumber(value) ? String(value) : null
}

export function truncatedEdgesText(count: number): string {
  return `${count} more connections were left off the map.`
}

export function networkModel(
  nodes: readonly NetworkNode[],
  rows: readonly EdgeRow[],
  layer: Record<string, unknown>,
  fixed: string | undefined
): NetworkModel {
  const places = new Map<string, Point>()
  for (const node of nodes) {
    if (!places.has(node.id)) places.set(node.id, project(node.lon, node.lat))
  }
  const edges: NetworkEdge[] = []
  let unlocated = 0
  let truncated = 0
  for (const row of rows) {
    const sourceId = idText(row.source)
    const targetId = idText(row.target)
    const from = sourceId === null ? undefined : places.get(sourceId)
    const to = targetId === null ? undefined : places.get(targetId)
    if (!from || !to) {
      unlocated++
    } else if (edges.length >= maxEdges) {
      truncated++
    } else {
      edges.push({ key: `${row.frameIndex}:${row.row}`, from, to })
    }
  }
  return {
    edges,
    unlocated,
    truncated,
    color: fixed ?? null,
    lineWidth: isNumber(layer.lineWidth) ? layer.lineWidth : defaultLineWidth,
    arrow: arrowOf(layer.arrow),
  }
}
