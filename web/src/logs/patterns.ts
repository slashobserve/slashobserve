import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, isRetryable, unwrap } from "@/api/errors"
import { exploreKeys, type LogPatternsParams } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { panelRequestLimiter } from "@/dashboards/limiter"
import { requestZone } from "@/lib/time"

import { appendKql, parseKql } from "./kql"
import { splitForm } from "./query-form"
import { isSearchFamily } from "./search"

export type LogPatterns = components["schemas"]["LogPatterns"]
export type LogPattern = components["schemas"]["LogPattern"]
export type PatternMode = "in" | "out"
export type PatternOrder = "count" | "newest"
export type PatternPart = { text: string; wildcard: boolean }

export const defaultPatternLimit = 20
export const maxPatternLimit = 100
const firstShown = 9
const minorShare = 0.01
const wildcard = "<*>"
const hasWord = /[\p{L}\p{N}]/u
const clickhouseTail = /\b(?:GROUP\s+BY|HAVING|ORDER\s+BY|LIMIT)\b/i
const clickhouseWhere = /\bWHERE\b/i

async function logPatternsRequest(
  uid: string,
  { query, from, to, limit }: LogPatternsParams,
  signal?: AbortSignal
): Promise<LogPatterns> {
  const timezone = requestZone("browser")
  return panelRequestLimiter.run(
    () =>
      api
        .POST("/datasources/{uid}/log-patterns", {
          params: { path: { uid } },
          body: {
            query,
            from,
            to,
            limit,
            ...(timezone !== undefined && { timezone }),
          },
          signal,
        })
        .then(unwrap),
    signal
  )
}

export function logPatternsQuery(uid: string, params: LogPatternsParams) {
  return queryOptions({
    queryKey: exploreKeys.patterns(uid, params),
    queryFn: ({ signal }) => logPatternsRequest(uid, params, signal),
    staleTime: 30_000,
    gcTime: 60_000,
    retry: (count, error) => count < 1 && isRetryable(error),
    refetchOnWindowFocus: false,
  })
}

export function patternsErrorText(error: unknown): {
  text: string
  retry: boolean
} {
  if (error instanceof ApiError && error.code === "query_invalid") {
    return {
      text: "Patterns need a log query that selects lines.",
      retry: false,
    }
  }
  return { text: "The patterns could not be computed.", retry: true }
}

export function patternParts(pattern: string): PatternPart[] {
  return pattern.split(wildcard).flatMap((text, index) => {
    const parts: PatternPart[] = []
    if (index > 0) parts.push({ text: wildcard, wildcard: true })
    if (text !== "") parts.push({ text, wildcard: false })
    return parts
  })
}

function escapeRegex(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
}

export function patternRegex(pattern: string): string {
  return patternParts(pattern)
    .map((part) => (part.wildcard ? ".*?" : escapeRegex(part.text)))
    .join("")
}

function patternTerms(
  pattern: string,
  source: LogPatterns["source"]
): string[] {
  const terms =
    source === "elasticsearch"
      ? pattern.split(/\s+/)
      : patternParts(pattern)
          .filter((part) => !part.wildcard)
          .map((part) => part.text.trim())
  return terms.filter((term) => hasWord.test(term))
}

function sqlString(value: string): string {
  return `'${value.replace(/[\\']/g, "\\$&")}'`
}

function lokiPatternFilter(
  query: string,
  regex: string,
  mode: PatternMode
): string | null {
  const text = query.trim()
  if (!text.startsWith("{")) return null
  const stage = `${mode === "in" ? "|~" : "!~"} ${JSON.stringify(regex)}`
  return text.includes(stage) ? null : `${text} ${stage}`
}

function clickhousePatternFilter(
  query: string,
  regex: string,
  mode: PatternMode
): string | null {
  const where = clickhouseWhere.exec(query)
  if (where === null) return null
  if ((query.match(/\bSELECT\b/gi) ?? []).length !== 1) return null
  const conditionStart = where.index + where[0].length
  const rest = query.slice(conditionStart)
  const tail = clickhouseTail.exec(rest)
  const conditionEnd = tail ? tail.index : rest.length
  const condition = rest.slice(0, conditionEnd).trim()
  const clause = `${mode === "out" ? "NOT " : ""}match(Body, ${sqlString(regex)})`
  if (condition.includes(clause)) return null
  const raw = rest.slice(0, conditionEnd)
  const trailing = raw.slice(raw.trimEnd().length)
  return `${query.slice(0, conditionStart)} (${condition}) AND ${clause}${trailing}${rest.slice(conditionEnd)}`
}

function joinKql(body: string, clause: string): string {
  const trimmed = body.trim()
  if (trimmed === "") return clause
  const { node, error } = parseKql(trimmed)
  return error !== null || node?.kind === "or"
    ? `(${trimmed}) and ${clause}`
    : `${trimmed} and ${clause}`
}

function searchPatternFilter(
  query: string,
  terms: string[],
  mode: PatternMode
): string | null {
  const { form, body, offset } = splitForm(query)
  if (form !== "kql") return null
  const matches = terms.map((term) => appendKql("", "message", term, false))
  const all = matches.join(" and ")
  if (mode === "in") {
    return body.includes(all)
      ? null
      : query.slice(0, offset) + joinKql(body, all)
  }
  const clause = `not ${matches.length === 1 ? all : `(${all})`}`
  return body.includes(clause)
    ? null
    : query.slice(0, offset) + joinKql(body, clause)
}

export function withPatternFilter(
  type: string,
  query: string,
  pattern: string,
  mode: PatternMode,
  source: LogPatterns["source"]
): string | null {
  const terms = patternTerms(pattern, source)
  if (terms.length === 0) return null
  if (isSearchFamily(type)) return searchPatternFilter(query, terms, mode)
  const regex = patternRegex(pattern)
  if (type === "loki") return lokiPatternFilter(query, regex, mode)
  if (type === "clickhouse") return clickhousePatternFilter(query, regex, mode)
  return null
}

export function patternShare(count: number, total: number): number {
  return total <= 0 ? 0 : count / total
}

export function shareText(share: number): string {
  const percent = share * 100
  return percent < 10 ? `${percent.toFixed(1)}%` : `${Math.round(percent)}%`
}

export function patternsHeading(result: LogPatterns): string {
  const total =
    result.lines_sampled > 0 ? result.lines_sampled : result.lines_matched
  const covered = result.patterns.reduce((sum, item) => sum + item.count, 0)
  const rounded = Math.round(patternShare(covered, total) * 100)
  const percent = covered < total ? Math.min(rounded, 99) : rounded
  const count = result.patterns.length
  const verb = count === 1 ? "pattern covers" : "patterns cover"
  return `${count} ${verb} ${percent}% of ${total.toLocaleString("en")} documents. Numbers and ids become ${wildcard}.`
}

export const patternsNote =
  "Patterns come from Loki when its pattern ingester is on; otherwise they are grouped here from the sampled lines, and the counts are for the sample only."

export function smallPatterns(
  patterns: readonly LogPattern[],
  linesMatched: number
): { shown: LogPattern[]; folded: LogPattern[] } {
  const shown: LogPattern[] = []
  const folded: LogPattern[] = []
  patterns.forEach((item, index) => {
    const major =
      index < firstShown || patternShare(item.count, linesMatched) >= minorShare
    ;(major ? shown : folded).push(item)
  })
  return { shown, folded }
}

export function patternsFooter(
  shown: number,
  total: number,
  folded: number
): string {
  return `${shown} of ${total} patterns shown, ${folded} cover under 1% each`
}

function lastTime(item: LogPattern): number {
  const time = Date.parse(item.last)
  return Number.isNaN(time) ? Number.NEGATIVE_INFINITY : time
}

export function sortPatterns(
  patterns: readonly LogPattern[],
  order: PatternOrder
): LogPattern[] {
  if (order === "count") return [...patterns]
  return [...patterns].sort(
    (a, b) => lastTime(b) - lastTime(a) || b.count - a.count
  )
}

function bucketSum(buckets: readonly { count: number }[]): number {
  return buckets.reduce((sum, bucket) => sum + bucket.count, 0)
}

export function trendChange(
  trend: readonly { count: number }[]
): string | null {
  const third = Math.floor(trend.length / 3)
  if (third === 0) return null
  const before = bucketSum(trend.slice(0, third))
  const after = bucketSum(trend.slice(-third))
  if (before === 0) return after === 0 ? null : "new"
  const percent = Math.round(((after - before) / before) * 100)
  return percent > 0 ? `+${percent}%` : `${percent}%`
}

export function patternCountText(result: LogPatterns): string {
  const count = result.patterns.length
  return result.truncated ? `${count}+` : String(count)
}
