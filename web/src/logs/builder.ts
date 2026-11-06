import {
  flatClauses,
  kqlText,
  needsQuotes,
  parseKql,
  valueNode,
  type KqlNode,
  type RangeOp,
} from "./kql"
import { formOf } from "./query-form"

export type BuilderOp =
  | "is"
  | "is not"
  | "is one of"
  | "exists"
  | "gt"
  | "gte"
  | "lt"
  | "lte"
  | "contains"

export type BuilderRow = {
  id: string
  field: string
  op: BuilderOp
  value: string
  values: string[]
}

const rangeOps: Record<"gt" | "gte" | "lt" | "lte", RangeOp> = {
  gt: ">",
  gte: ">=",
  lt: "<",
  lte: "<=",
}

const opOfRange: Record<RangeOp, "gt" | "gte" | "lt" | "lte"> = {
  ">": "gt",
  ">=": "gte",
  "<": "lt",
  "<=": "lte",
}

function rowNode(row: BuilderRow): KqlNode {
  const { field, value } = row
  switch (row.op) {
    case "is":
      return { kind: "match", field, ...valueNode(value) }
    case "is not":
      return {
        kind: "not",
        item: { kind: "match", field, ...valueNode(value) },
      }
    case "is one of":
      return { kind: "in", field, values: row.values, op: "or" }
    case "exists":
      return { kind: "exists", field }
    case "contains":
      return { kind: "free", ...valueNode(value) }
    case "gt":
    case "gte":
    case "lt":
    case "lte":
      return {
        kind: "range",
        field,
        op: rangeOps[row.op],
        value,
        quoted: needsQuotes(value),
      }
  }
}

export function textOf(rows: BuilderRow[]): string {
  if (rows.length === 0) return ""
  const nodes = rows.map(rowNode)
  return kqlText(nodes.length === 1 ? nodes[0] : { kind: "and", items: nodes })
}

function blankRow(id: string): BuilderRow {
  return { id, field: "", op: "is", value: "", values: [] }
}

function rowOf(node: KqlNode, id: string): BuilderRow | null {
  const row = blankRow(id)
  switch (node.kind) {
    case "match":
      return { ...row, field: node.field, value: node.value }
    case "free":
      return { ...row, op: "contains", value: node.value }
    case "exists":
      return { ...row, field: node.field, op: "exists" }
    case "range":
      return {
        ...row,
        field: node.field,
        op: opOfRange[node.op],
        value: node.value,
      }
    case "in":
      if (node.op === "and" && node.values.length > 1) return null
      return {
        ...row,
        field: node.field,
        op: "is one of",
        values: node.values,
      }
    case "not":
      return node.item.kind === "match"
        ? {
            ...row,
            field: node.item.field,
            op: "is not",
            value: node.item.value,
          }
        : null
    default:
      return null
  }
}

function comparable(node: KqlNode): KqlNode {
  switch (node.kind) {
    case "and":
    case "or":
      return { ...node, items: node.items.map(comparable) }
    case "not":
    case "group":
      return { ...node, item: comparable(node.item) }
    case "match":
    case "range":
    case "free":
      return {
        ...node,
        quoted:
          node.quoted && (needsQuotes(node.value) || node.value.includes("*")),
      }
    case "exists":
    case "in":
      return node
  }
}

function sameQuery(a: KqlNode | null, b: KqlNode | null): boolean {
  if (a === null || b === null) return a === b
  return JSON.stringify(comparable(a)) === JSON.stringify(comparable(b))
}

export function rowsOf(text: string): BuilderRow[] | null {
  const { node, error } = parseKql(text)
  if (error !== null) return null
  if (node === null) return []
  const clauses = flatClauses(node)
  if (clauses === null) return null
  const rows: BuilderRow[] = []
  for (const [index, clause] of clauses.entries()) {
    const row = rowOf(clause, String(index))
    if (row === null) return null
    rows.push(row)
  }
  return sameQuery(node, parseKql(textOf(rows)).node) ? rows : null
}

export function canBuild(text: string): boolean {
  return formOf(text) === "kql" && rowsOf(text) !== null
}

export function withoutField(text: string, field: string): string | null {
  const rows = rowsOf(text)
  if (rows === null) return null
  return textOf(rows.filter((row) => row.field !== field))
}

export function rowLabel(row: BuilderRow): string {
  switch (row.op) {
    case "is":
      return `${row.field} : ${row.value}`
    case "is not":
      return `not ${row.field} : ${row.value}`
    case "is one of":
      return `${row.field} : ${row.values.join(" or ")}`
    case "exists":
      return `${row.field} exists`
    case "contains":
      return row.value
    case "gt":
    case "gte":
    case "lt":
    case "lte":
      return `${row.field} ${rangeOps[row.op]} ${row.value}`
  }
}
