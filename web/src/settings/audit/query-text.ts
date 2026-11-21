import {
  appendKql,
  escapeKql,
  flatClauses,
  kqlText,
  parseKql,
  type KqlNode,
} from "@/logs/kql"
import { splitForm } from "@/logs/query-form"

export type FacetField = { name: string; alias: string | null }

export type TermState = "include" | "exclude" | null

type Found = { clauses: KqlNode[]; indexes: number[]; state: TermState }

function namesOf(field: FacetField): string[] {
  return field.alias === null ? [field.name] : [field.name, field.alias]
}

function matches(node: KqlNode, names: string[], value: string): boolean {
  return (
    node.kind === "match" &&
    names.includes(node.field) &&
    !node.wildcard &&
    node.value === value
  )
}

function stateOf(node: KqlNode, names: string[], value: string): TermState {
  if (matches(node, names, value)) return "include"
  if (node.kind === "not" && matches(node.item, names, value)) return "exclude"
  return null
}

function readKql(text: string): { prefix: string; body: string } | null {
  const { form, body, offset } = splitForm(text)
  if (form !== "kql") return null
  if (parseKql(body).error !== null) return null
  return { prefix: text.slice(0, offset), body }
}

function find(body: string, field: FacetField, value: string): Found | null {
  const { node } = parseKql(body)
  if (node === null) return null
  const clauses = flatClauses(node)
  if (clauses === null) return null
  const names = namesOf(field)
  const indexes = clauses.flatMap((clause, index) =>
    stateOf(clause, names, value) === null ? [] : [index]
  )
  if (indexes.length === 0) return null
  return { clauses, indexes, state: stateOf(clauses[indexes[0]], names, value) }
}

export function canEditTerms(text: string): boolean {
  return readKql(text) !== null
}

export function termState(
  text: string,
  field: FacetField,
  value: string
): TermState {
  const kql = readKql(text)
  if (kql === null) return null
  return find(kql.body, field, value)?.state ?? null
}

function term(field: string, value: string, exclude: boolean): KqlNode {
  const match: KqlNode = {
    kind: "match",
    field,
    value,
    quoted: true,
    wildcard: false,
  }
  return exclude ? { kind: "not", item: match } : match
}

function printed(clauses: KqlNode[]): string {
  return clauses.map(kqlText).join(" and ")
}

export function toggleTerm(
  text: string,
  field: FacetField,
  value: string,
  exclude: boolean
): string | null {
  const kql = readKql(text)
  if (kql === null) return null
  const wanted: TermState = exclude ? "exclude" : "include"
  const found = find(kql.body, field, value)
  if (found === null) {
    return kql.prefix + appendKql(kql.body, field.name, value, exclude)
  }
  const [first, ...copies] = found.indexes
  const dropped = found.state === wanted ? found.indexes : copies
  const clauses = found.clauses.map((clause, index) =>
    index === first ? term(field.name, value, exclude) : clause
  )
  return (
    kql.prefix + printed(clauses.filter((_, index) => !dropped.includes(index)))
  )
}

export function requestQuery(requestId: string): string {
  return `request_id:${escapeKql(requestId)}`
}

export function runQuery(runId: string): string {
  return `run_id:${escapeKql(runId)}`
}
