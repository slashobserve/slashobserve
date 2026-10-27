import { styleForSlot } from "../colors"
import {
  displayValue,
  type DisplayValue,
  type FieldResolver,
} from "../field-config"
import { uniqueKey } from "../frames"
import { thresholdColor } from "../thresholds"
import type { PreparedField, PreparedFrame } from "../transforms"
import { rowCount } from "../transforms/names"

export const maxNodes = 300
export const maxEdges = 1000
export const minThickness = 1
export const maxThickness = 8

export const missingFramesReason =
  "A node graph needs an edges query with id, source and target columns, or a nodes query with an id column."

const criticalColor = "var(--threshold-critical)"
const colorSyntax = /^(#[0-9a-f]{3,8}|[a-z]+|(rgb|hsl)a?\([\d\s.,%/-]+\))$/i

export type GraphStat = { name: string; display: DisplayValue }
export type GraphArc = { name: string; fraction: number; color: string }
export type GraphDetail = { name: string; text: string }

export type GraphNode = {
  id: string
  title: string
  subtitle: string
  mainStat: GraphStat | null
  secondaryStat: GraphStat | null
  arcs: GraphArc[]
  details: GraphDetail[]
  color: string | null
  loops: number
}

export type GraphEdge = {
  id: string
  source: string
  target: string
  mainStat: GraphStat | null
  secondaryStat: GraphStat | null
  details: GraphDetail[]
  thickness: number
  color: string | null
}

export type GraphModel =
  | { reason: string }
  | {
      reason: null
      nodes: GraphNode[]
      edges: GraphEdge[]
      note: string | null
    }

type Columns = {
  frame: PreparedFrame
  named: Map<string, PreparedField>
  rows: number
}

type Kind = "nodes" | "edges"

function columnsOf(frame: PreparedFrame): Columns {
  const named = new Map<string, PreparedField>()
  for (const field of frame.fields) {
    const key = field.name.toLowerCase()
    if (!named.has(key)) named.set(key, field)
  }
  return { frame, named, rows: rowCount(frame) }
}

function kindOf({ frame, named }: Columns): Kind | null {
  if (frame.name?.toLowerCase() === "nodes" && named.has("id")) return "nodes"
  if (named.has("source") && named.has("target")) return "edges"
  return named.has("id") ? "nodes" : null
}

function isAbsent(value: unknown): boolean {
  return value === null || value === undefined || value === ""
}

function cellOf(columns: Columns, name: string, row: number): unknown {
  return columns.named.get(name)?.values[row] ?? null
}

function textOf(columns: Columns, name: string, row: number): string {
  const value = cellOf(columns, name, row)
  return isAbsent(value) ? "" : String(value)
}

function labelOf(
  fields: FieldResolver,
  frame: PreparedFrame,
  field: PreparedField,
  fallback: string
): string {
  return fields.configFor(frame, field).displayName === undefined
    ? fallback
    : fields.displayName(frame, field)
}

function statOf(
  columns: Columns,
  fields: FieldResolver,
  name: string,
  fallback: string,
  row: number
): GraphStat | null {
  const field = columns.named.get(name)
  const value = field?.values[row]
  if (!field || isAbsent(value)) return null
  return {
    name: labelOf(fields, columns.frame, field, fallback),
    display: displayValue(value, fields.configFor(columns.frame, field)),
  }
}

function prefixed(columns: Columns, prefix: string): PreparedField[] {
  return columns.frame.fields.filter((field) =>
    field.name.toLowerCase().startsWith(prefix)
  )
}

function detailsOf(
  columns: Columns,
  fields: FieldResolver,
  row: number
): GraphDetail[] {
  return prefixed(columns, "detail__").flatMap((field) => {
    const value = field.values[row]
    if (isAbsent(value)) return []
    const config = fields.configFor(columns.frame, field)
    return [
      {
        name: labelOf(
          fields,
          columns.frame,
          field,
          field.name.slice("detail__".length)
        ),
        text: displayValue(value, config).text,
      },
    ]
  })
}

function arcsOf(
  columns: Columns,
  fields: FieldResolver,
  row: number
): GraphArc[] {
  const drawn = prefixed(columns, "arc__").flatMap((field, index) => {
    const value = field.values[row]
    if (typeof value !== "number" || !(value > 0)) return []
    const shown = displayValue(value, fields.configFor(columns.frame, field))
    return [
      {
        name: labelOf(
          fields,
          columns.frame,
          field,
          field.name.slice("arc__".length)
        ),
        value,
        color: shown.color ?? styleForSlot(index).color,
      },
    ]
  })
  const total = drawn.reduce((sum, arc) => sum + arc.value, 0)
  const scale = Math.max(total, 1)
  return drawn.map(({ name, value, color }) => ({
    name,
    fraction: value / scale,
    color,
  }))
}

function colorOf(columns: Columns, row: number): string | null {
  const color = textOf(columns, "color", row).trim()
  if (colorSyntax.test(color)) return thresholdColor(color)
  const highlighted = cellOf(columns, "highlighted", row)
  return highlighted === true || highlighted === 1 || highlighted === "true"
    ? criticalColor
    : null
}

function readNode(
  columns: Columns,
  fields: FieldResolver,
  row: number
): GraphNode | null {
  const id = textOf(columns, "id", row)
  if (id === "") return null
  return {
    id,
    title: textOf(columns, "title", row) || id,
    subtitle: textOf(columns, "subtitle", row),
    mainStat: statOf(columns, fields, "mainstat", "Main stat", row),
    secondaryStat: statOf(
      columns,
      fields,
      "secondarystat",
      "Secondary stat",
      row
    ),
    arcs: arcsOf(columns, fields, row),
    details: detailsOf(columns, fields, row),
    color: colorOf(columns, row),
    loops: 0,
  }
}

function thicknessOf(columns: Columns, row: number): number {
  const value = Number(cellOf(columns, "thickness", row))
  if (!Number.isFinite(value)) return minThickness
  return Math.min(maxThickness, Math.max(minThickness, value))
}

function readEdge(
  columns: Columns,
  fields: FieldResolver,
  row: number,
  ids: Map<string, number>
): GraphEdge | null {
  const source = textOf(columns, "source", row)
  const target = textOf(columns, "target", row)
  if (source === "" || target === "") return null
  const given = textOf(columns, "id", row)
  return {
    id: uniqueKey(ids, given === "" ? `${source}->${target}` : given),
    source,
    target,
    mainStat: statOf(columns, fields, "mainstat", "Main stat", row),
    secondaryStat: statOf(
      columns,
      fields,
      "secondarystat",
      "Secondary stat",
      row
    ),
    details: detailsOf(columns, fields, row),
    thickness: thicknessOf(columns, row),
    color: colorOf(columns, row),
  }
}

function endpointNode(id: string): GraphNode {
  return {
    id,
    title: id,
    subtitle: "",
    mainStat: null,
    secondaryStat: null,
    arcs: [],
    details: [],
    color: null,
    loops: 0,
  }
}

function nodeFor(nodes: Map<string, GraphNode>, id: string): GraphNode {
  let node = nodes.get(id)
  if (!node) {
    node = endpointNode(id)
    nodes.set(id, node)
  }
  return node
}

export function graphModel(
  frames: PreparedFrame[],
  fields: FieldResolver
): GraphModel {
  const classified = frames.flatMap((frame) => {
    const columns = columnsOf(frame)
    const kind = kindOf(columns)
    return kind === null ? [] : [{ columns, kind }]
  })
  if (classified.length === 0) return { reason: missingFramesReason }

  const nodes = new Map<string, GraphNode>()
  const edges: GraphEdge[] = []
  const edgeIds = new Map<string, number>()
  for (const { columns, kind } of classified) {
    for (let row = 0; row < columns.rows; row++) {
      if (kind === "nodes") {
        const node = readNode(columns, fields, row)
        if (node && !nodes.has(node.id)) nodes.set(node.id, node)
        continue
      }
      const edge = readEdge(columns, fields, row, edgeIds)
      if (edge) edges.push(edge)
    }
  }

  const connections: GraphEdge[] = []
  for (const edge of edges) {
    const source = nodeFor(nodes, edge.source)
    nodeFor(nodes, edge.target)
    if (edge.source === edge.target) source.loops++
    else connections.push(edge)
  }

  const degree = new Map<string, number>()
  for (const { source, target } of connections) {
    degree.set(source, (degree.get(source) ?? 0) + 1)
    degree.set(target, (degree.get(target) ?? 0) + 1)
  }
  const all = [...nodes.values()]
  const kept =
    all.length <= maxNodes
      ? all
      : all
          .map((node, index) => ({ node, index }))
          .sort(
            (a, b) =>
              (degree.get(b.node.id) ?? 0) - (degree.get(a.node.id) ?? 0) ||
              a.index - b.index
          )
          .slice(0, maxNodes)
          .sort((a, b) => a.index - b.index)
          .map(({ node }) => node)
  const keptIds = new Set(kept.map((node) => node.id))
  const reachable = connections.filter(
    ({ source, target }) => keptIds.has(source) && keptIds.has(target)
  )

  const notes: string[] = []
  if (all.length > kept.length) {
    notes.push(`Showing ${kept.length} of ${all.length} services.`)
  }
  if (reachable.length > maxEdges) {
    notes.push(`Showing ${maxEdges} of ${reachable.length} connections.`)
  }
  return {
    reason: null,
    nodes: kept,
    edges: reachable.slice(0, maxEdges),
    note: notes.length > 0 ? notes.join(" ") : null,
  }
}

function statRow(stat: GraphStat | null): GraphDetail[] {
  return stat ? [{ name: stat.name, text: stat.display.text }] : []
}

export function nodeRows(node: GraphNode): GraphDetail[] {
  return [
    ...statRow(node.mainStat),
    ...statRow(node.secondaryStat),
    ...node.arcs.map((arc) => ({
      name: arc.name,
      text: `${(arc.fraction * 100).toFixed(1)}%`,
    })),
    ...node.details,
    ...(node.loops > 0
      ? [{ name: "Self calls", text: String(node.loops) }]
      : []),
  ]
}

export function edgeRows(edge: GraphEdge): GraphDetail[] {
  return [
    ...statRow(edge.mainStat),
    ...statRow(edge.secondaryStat),
    ...edge.details,
  ]
}
