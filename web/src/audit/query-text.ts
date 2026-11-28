import {
  escapeKql,
  flatClauses,
  parseKql,
  parseKqlClauses,
  type KqlClause,
  type KqlNode,
} from "@/logs/kql"
import { splitForm } from "@/logs/query-form"

import type { AuditEvent } from "./stream"

export type FacetField = { name: string; aliases: readonly string[] }

export type TermState = "include" | "exclude" | null

type Source = { prefix: string; body: string; clauses: KqlClause[] }

function namesOf(field: FacetField): string[] {
  return [field.name, ...field.aliases]
}

export function suggestedNames(fields: readonly FacetField[]): string[] {
  return fields.flatMap(namesOf)
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

function readKql(text: string): Source | null {
  const { form, body, offset } = splitForm(text)
  if (form !== "kql") return null
  const clauses = parseKqlClauses(body)
  if (clauses === null) return null
  return { prefix: text.slice(0, offset), body, clauses }
}

function stateAt(
  clause: KqlClause,
  field: FacetField,
  value: string
): TermState {
  const flat = flatClauses(clause.node)?.[0]
  return flat === undefined ? null : stateOf(flat, namesOf(field), value)
}

function foundAt(
  clauses: KqlClause[],
  field: FacetField,
  value: string
): number[] {
  return clauses.flatMap((clause, index) =>
    stateAt(clause, field, value) === null ? [] : [index]
  )
}

export function canEditTerms(text: string): boolean {
  return readKql(text) !== null
}

export function termState(
  text: string,
  field: FacetField,
  value: string
): TermState {
  const source = readKql(text)
  if (source === null) return null
  const [first] = foundAt(source.clauses, field, value)
  return first === undefined
    ? null
    : stateAt(source.clauses[first], field, value)
}

function termText(field: string, value: string, exclude: boolean): string {
  const clause = `${field}:${escapeKql(value)}`
  return exclude ? `not ${clause}` : clause
}

function appended(body: string, clause: string): string {
  const trimmed = body.trim()
  if (trimmed === "" || trimmed === "*") return clause
  const wrapped = parseKql(trimmed).node?.kind === "or"
  return wrapped ? `(${trimmed}) and ${clause}` : `${trimmed} and ${clause}`
}

function withoutClause(body: string, clauses: KqlClause[], index: number) {
  const clause = clauses[index]
  if (clauses.length === 1) return body.slice(0, clause.start)
  if (index === 0) {
    return body.slice(0, clause.start) + body.slice(clauses[1].start)
  }
  return body.slice(0, clauses[index - 1].end) + body.slice(clause.end)
}

function replacingClause(body: string, clause: KqlClause, text: string) {
  return body.slice(0, clause.start) + text + body.slice(clause.end)
}

function removeCopies(
  body: string,
  field: FacetField,
  value: string,
  keep: number
): string {
  let current = body
  let kept = keep
  for (;;) {
    const clauses = parseKqlClauses(current)
    if (clauses === null) return current
    const found = foundAt(clauses, field, value).filter(
      (index) => index !== kept
    )
    if (found.length === 0) return current
    const last = found[found.length - 1]
    current = withoutClause(current, clauses, last)
    if (last < kept) kept -= 1
  }
}

export function toggleTerm(
  text: string,
  field: FacetField,
  value: string,
  exclude: boolean
): string | null {
  const source = readKql(text)
  if (source === null) return null
  const { prefix, body, clauses } = source
  const found = foundAt(clauses, field, value)
  if (found.length === 0) {
    return prefix + appended(body, termText(field.name, value, exclude))
  }
  const [first] = found
  const wanted: TermState = exclude ? "exclude" : "include"
  if (stateAt(clauses[first], field, value) === wanted) {
    return prefix + removeCopies(body, field, value, -1).trimEnd()
  }
  const flipped = replacingClause(
    body,
    clauses[first],
    termText(field.name, value, exclude)
  )
  return prefix + removeCopies(flipped, field, value, first)
}

export type WrapMemory = { before: string; after: string }

function wrapsOnAdd(text: string): boolean {
  const source = readKql(text)
  if (source === null) return false
  const trimmed = source.body.trim()
  return trimmed !== "" && parseKql(trimmed).node?.kind === "or"
}

export function toggleTermRemembering(
  text: string,
  field: FacetField,
  value: string,
  exclude: boolean,
  memory: WrapMemory | null
): { text: string; memory: WrapMemory | null } | null {
  const next = toggleTerm(text, field, value, exclude)
  if (next === null) return null
  if (
    memory !== null &&
    memory.after === text &&
    toggleTerm(memory.before, field, value, exclude) === text
  ) {
    return { text: memory.before, memory: null }
  }
  const wrapped = termState(text, field, value) === null && wrapsOnAdd(text)
  return { text: next, memory: wrapped ? { before: text, after: next } : null }
}

export function requestQuery(requestId: string): string {
  return `request_id:${escapeKql(requestId)}`
}

export function runQuery(runId: string): string {
  return `run_id:${escapeKql(runId)}`
}

function filled(value: string | null | undefined): value is string {
  return value !== null && value !== undefined && value !== ""
}

export function actorQuery(event: AuditEvent): string {
  if (filled(event.actor_id)) return `actor_id:${escapeKql(event.actor_id)}`
  if (filled(event.actor_name)) {
    return `actor_name:${escapeKql(event.actor_name)}`
  }
  return `actor_kind:${escapeKql(event.actor_kind)}`
}

export function targetQuery(event: AuditEvent): string | null {
  const { target_kind: kind, target_id: id } = event
  if (filled(kind) && filled(id)) return `target:${escapeKql(`${kind}/${id}`)}`
  if (filled(id)) return `target_id:${escapeKql(id)}`
  if (filled(kind)) return `target_kind:${escapeKql(kind)}`
  return null
}
