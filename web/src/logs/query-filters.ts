import { matcherInsert } from "@/panel-editor/insert-text"
import type { Labels } from "@/panels/logs/logs-model"

import { perLineLokiLabels } from "./context-query"

export type FilterTarget =
  "loki-stream" | "loki-line" | "clickhouse-column" | "clickhouse-attribute"

type Rewrite = { text: string } | { reason: "present" | "unsupported" }

const present: Rewrite = { reason: "present" }
const unsupported: Rewrite = { reason: "unsupported" }

const clickhouseColumns = new Set([
  "Timestamp",
  "SeverityText",
  "Body",
  "ServiceName",
  "TraceId",
  "SpanId",
])

const lokiLabel = /^[A-Za-z_][A-Za-z0-9_]*$/
const matcherStart = /^([A-Za-z_][A-Za-z0-9_]*)\s*(=~|!~|!=|=)/
const quoteCharacters = ['"', "`"]

export function isStreamLabel(
  name: string,
  labels: Labels,
  indexed: readonly string[] | undefined
): boolean {
  if (!(name in labels) || perLineLokiLabels.has(name)) return false
  return indexed === undefined || indexed.length === 0 || indexed.includes(name)
}

export function filterTarget(
  type: "loki" | "clickhouse",
  name: string,
  isStreamLabel: boolean
): FilterTarget {
  if (type === "loki") return isStreamLabel ? "loki-stream" : "loki-line"
  return clickhouseColumns.has(name)
    ? "clickhouse-column"
    : "clickhouse-attribute"
}

function matcher(
  type: "loki" | "clickhouse",
  expression: string,
  value: string,
  negate: boolean
): string {
  const equal = matcherInsert(type, expression, value)
  if (!negate) return equal
  const [operator, negated] =
    type === "clickhouse" ? [" = ", " != "] : ["=", "!="]
  return expression + negated + equal.slice(expression.length + operator.length)
}

function afterQuoted(text: string, at: number): number {
  const quote = text[at]
  let index = at + 1
  while (index < text.length) {
    if (quote === '"' && text[index] === "\\") {
      index += 2
    } else if (text[index] === quote) {
      return index + 1
    } else {
      index++
    }
  }
  return text.length
}

function selectorSpan(query: string): { start: number; end: number } | null {
  let index = 0
  while (index < query.length) {
    if (quoteCharacters.includes(query[index])) {
      index = afterQuoted(query, index)
    } else if (query[index] === "{") {
      let end = index + 1
      while (end < query.length && query[end] !== "}") {
        end = quoteCharacters.includes(query[end])
          ? afterQuoted(query, end)
          : end + 1
      }
      return end < query.length ? { start: index, end } : null
    } else {
      index++
    }
  }
  return null
}

type SelectorMatcher = {
  start: number
  end: number
  label: string
  operator: string
}

function selectorMatchers(body: string): SelectorMatcher[] {
  const found: SelectorMatcher[] = []
  const close = (from: number, to: number) => {
    const text = body.slice(from, to)
    const start = from + (text.length - text.trimStart().length)
    const end = from + text.trimEnd().length
    if (end <= start) return
    const parsed = matcherStart.exec(body.slice(start, end))
    found.push({
      start,
      end,
      label: parsed?.[1] ?? "",
      operator: parsed?.[2] ?? "",
    })
  }
  let from = 0
  let index = 0
  while (index < body.length) {
    if (quoteCharacters.includes(body[index])) {
      index = afterQuoted(body, index)
    } else if (body[index] === ",") {
      close(from, index)
      from = index + 1
      index++
    } else {
      index++
    }
  }
  close(from, body.length)
  return found
}

function rewriteLokiStream(
  query: string,
  name: string,
  value: string,
  negate: boolean
): Rewrite {
  const span = selectorSpan(query)
  if (span === null) return unsupported
  const body = query.slice(span.start + 1, span.end)
  const fresh = matcher("loki", name, value, negate)
  const existing = selectorMatchers(body).find(
    (candidate) => candidate.label === name
  )
  let next: string
  if (existing !== undefined) {
    if (body.slice(existing.start, existing.end) === fresh) return present
    next = body.slice(0, existing.start) + fresh + body.slice(existing.end)
  } else {
    const head = body.trimEnd()
    const joiner = head === "" ? "" : head.endsWith(",") ? " " : ", "
    next = head + joiner + fresh + body.slice(head.length)
  }
  const selectsStreams = selectorMatchers(next).some(
    (candidate) => candidate.operator === "=" || candidate.operator === "=~"
  )
  if (!selectsStreams) return unsupported
  return {
    text: query.slice(0, span.start + 1) + next + query.slice(span.end),
  }
}

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
}

function rewriteLokiLine(
  query: string,
  name: string,
  value: string,
  negate: boolean
): Rewrite {
  if (!query.trimStart().startsWith("{")) return unsupported
  const stage = matcher("loki", name, value, negate)
  const held = new RegExp(`\\|\\s*${escapeRegExp(stage)}(?=\\s|\\||$)`)
  if (held.test(query)) return present
  return { text: `${query.trimEnd()} | ${stage}` }
}

function rewriteClickhouse(
  query: string,
  expression: string,
  value: string,
  negate: boolean
): Rewrite {
  if ((query.match(/\bselect\b/gi) ?? []).length !== 1) return unsupported
  const where = /\bwhere\b/i.exec(query)
  if (where === null) return unsupported
  const clauseStart = where.index + where[0].length
  const end = /\b(?:order\s+by|limit)\b/i.exec(query.slice(clauseStart))
  const clauseEnd = end === null ? query.length : clauseStart + end.index
  const head = query.slice(0, clauseEnd).trimEnd()
  const clause = head.slice(clauseStart)
  const condition = matcher("clickhouse", expression, value, negate)
  if (clause.includes(condition)) return present
  const kept = /\bor\b/i.test(clause) ? ` (${clause.trim()})` : clause
  return {
    text: `${query.slice(0, clauseStart)}${kept} AND ${condition}${query.slice(head.length)}`,
  }
}

function rewrite(
  type: "loki" | "clickhouse",
  target: FilterTarget,
  name: string,
  query: string,
  value: string,
  negate: boolean
): Rewrite {
  switch (target) {
    case "loki-stream":
    case "loki-line":
      if (type !== "loki" || !lokiLabel.test(name)) return unsupported
      return target === "loki-stream"
        ? rewriteLokiStream(query, name, value, negate)
        : rewriteLokiLine(query, name, value, negate)
    case "clickhouse-column":
      return type === "clickhouse"
        ? rewriteClickhouse(query, name, value, negate)
        : unsupported
    case "clickhouse-attribute":
      return type === "clickhouse" && !/['\\]/.test(name)
        ? rewriteClickhouse(query, `LogAttributes['${name}']`, value, negate)
        : unsupported
  }
}

export function withFilter(
  query: string,
  type: "loki" | "clickhouse",
  target: FilterTarget,
  name: string,
  value: string,
  negate: boolean
): string | null {
  const result = rewrite(type, target, name, query, value, negate)
  return "text" in result ? result.text : null
}
