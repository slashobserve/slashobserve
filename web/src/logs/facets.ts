import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, isRetryable, unwrap } from "@/api/errors"
import { exploreKeys, type LogFacetsParams } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { panelRequestLimiter } from "@/dashboards/limiter"
import { formatValue } from "@/lib/units"
import { requestZone } from "@/lib/time"

import { appendKql } from "./kql"
import { splitForm } from "./query-form"
import { isSearchFamily } from "./search"

type ServedFacets = components["schemas"]["LogFacets"]
export type FacetField = Omit<ServedFacets["fields"][number], "kind"> & {
  kind: string
}
export type LogFacets = Omit<ServedFacets, "fields"> & { fields: FacetField[] }
export type FacetValue = FacetField["values"][number]
export type FilterMode = "in" | "out"
export type FieldRow = { name: string; facet: FacetField | null }
export type FacetBasis = "exact" | "sampled" | "approximate"

export const topValues = 5
const facetsOpenKey = "slashobserve-logs-facets-open"

async function logFacetsRequest(
  uid: string,
  { query, from, to, limit, fields }: LogFacetsParams,
  signal?: AbortSignal
): Promise<LogFacets> {
  const timezone = requestZone("browser")
  return panelRequestLimiter.run(
    () =>
      api
        .POST("/datasources/{uid}/log-facets", {
          params: { path: { uid } },
          body: {
            query,
            from,
            to,
            limit,
            ...(timezone !== undefined && { timezone }),
            ...(fields !== undefined && { fields }),
          },
          signal,
        })
        .then(unwrap),
    signal
  )
}

export function logFacetsQuery(uid: string, params: LogFacetsParams) {
  return queryOptions({
    queryKey: exploreKeys.facets(uid, params),
    queryFn: ({ signal }) => logFacetsRequest(uid, params, signal),
    staleTime: 30_000,
    gcTime: 60_000,
    retry: (count, error) => count < 1 && isRetryable(error),
    refetchOnWindowFocus: false,
  })
}

export function facetsErrorText(error: unknown): {
  text: string
  retry: boolean
} {
  if (error instanceof ApiError && error.code === "query_invalid") {
    return { text: "Fields need a log query.", retry: false }
  }
  return { text: "The fields could not be counted.", retry: true }
}

export function facetCountText(field: FacetField, value: FacetValue): string {
  if (field.unit === "bytes") return formatValue(value.count, "bytes")
  return value.count.toLocaleString("en")
}

export function facetBasis(facets: LogFacets, field: FacetField): FacetBasis {
  if (field.exact) return "exact"
  return facets.sampled ? "sampled" : "approximate"
}

export function basisNote(facets: LogFacets, field: FacetField): string {
  const basis = facetBasis(facets, field)
  if (basis === "sampled") {
    return `Counted over the newest ${facets.lines_sampled.toLocaleString("en")} lines.`
  }
  if (basis === "approximate") return "The counts are approximate."
  if (field.unit === "bytes") return "Counted in bytes from the stream index."
  return "Counted over every matching line."
}

export function barPercent(field: FacetField, value: FacetValue): number {
  const top = Math.max(0, ...field.values.map((item) => item.count))
  return top === 0 ? 0 : Math.round((value.count / top) * 100)
}

const kindBadges: Record<string, string> = {
  keyword: "k",
  text: "t",
  number: "#",
  date: "d",
  date_nanos: "d",
  boolean: "b",
  ip: "ip",
  flattened: "{}",
  label: "l",
  field: "f",
  metadata: "m",
  column: "c",
}

export function kindBadge(kind: string): string {
  return kindBadges[kind] ?? "?"
}

export function fieldsNote(
  type: string,
  index: string | undefined,
  count: number
): string {
  const fields =
    count === 1 ? "1 field" : `${count.toLocaleString("en")} fields`
  if (isSearchFamily(type)) {
    return `From the ${index ?? "index"} mapping, ${fields}`
  }
  if (type === "loki") return `From the stream labels, ${fields}`
  return `From the table columns, ${fields}`
}

export function fieldRows(
  names: readonly string[],
  facets: readonly FacetField[]
): FieldRow[] {
  const byName = new Map(facets.map((facet) => [facet.name, facet]))
  const all = new Set([...names, ...byName.keys()])
  return [...all]
    .sort((a, b) => a.localeCompare(b))
    .map((name) => ({ name, facet: byName.get(name) ?? null }))
}

export function splitFields(
  rows: readonly FieldRow[],
  selected: readonly string[],
  filter: string
): { selected: FieldRow[]; available: FieldRow[] } {
  const needle = filter.trim().toLowerCase()
  const shown = (row: FieldRow) => row.name.toLowerCase().includes(needle)
  const byName = new Map(rows.map((row) => [row.name, row]))
  const chosen = selected.map(
    (name) => byName.get(name) ?? { name, facet: null }
  )
  const chosenNames = new Set(selected)
  return {
    selected: chosen.filter(shown),
    available: rows.filter((row) => !chosenNames.has(row.name) && shown(row)),
  }
}

export function storedFacetsOpen(): boolean {
  try {
    return localStorage.getItem(facetsOpenKey) !== "false"
  } catch {
    return true
  }
}

export function storeFacetsOpen(open: boolean): void {
  try {
    localStorage.setItem(facetsOpenKey, String(open))
  } catch {
    return
  }
}

const columnName = /^[A-Za-z_][A-Za-z0-9_]*$/
const attributeName = /^LogAttributes\.(.+)$/
const clickhouseTail = /\b(?:GROUP\s+BY|HAVING|ORDER\s+BY|LIMIT)\b/i
const clickhouseWhere = /\bWHERE\b/i

function logqlString(value: string): string {
  return JSON.stringify(value)
}

function sqlString(value: string): string {
  return `'${value.replace(/[\\']/g, "\\$&")}'`
}

function lokiFilter(
  query: string,
  field: string,
  value: string,
  mode: FilterMode
): string | null {
  const text = query.trim()
  if (!text.startsWith("{") || !columnName.test(field)) return null
  const stage = `| ${field}${mode === "in" ? "=" : "!="}${logqlString(value)}`
  return text.includes(stage) ? null : `${text} ${stage}`
}

function clickhouseFilter(
  query: string,
  field: string,
  value: string,
  mode: FilterMode
): string | null {
  const attribute = attributeName.exec(field)
  const column = attribute
    ? `LogAttributes[${sqlString(attribute[1])}]`
    : columnName.test(field)
      ? field
      : null
  const where = clickhouseWhere.exec(query)
  if (column === null || where === null) return null
  if ((query.match(/\bSELECT\b/gi) ?? []).length !== 1) return null
  const conditionStart = where.index + where[0].length
  const rest = query.slice(conditionStart)
  const tail = clickhouseTail.exec(rest)
  const conditionEnd = tail ? tail.index : rest.length
  const condition = rest.slice(0, conditionEnd).trim()
  const clause = `${column} ${mode === "in" ? "=" : "!="} ${sqlString(value)}`
  if (condition.includes(clause)) return null
  const raw = rest.slice(0, conditionEnd)
  const trailing = raw.slice(raw.trimEnd().length)
  return `${query.slice(0, conditionStart)} (${condition}) AND ${clause}${trailing}${rest.slice(conditionEnd)}`
}

function searchFilter(
  query: string,
  field: string,
  value: string,
  mode: FilterMode
): string | null {
  const { form, body } = splitForm(query)
  if (form !== "kql") return null
  const clause = appendKql("", field, value, mode === "out")
  return body.includes(clause)
    ? null
    : appendKql(body, field, value, mode === "out")
}

export function filterQuery(
  type: string,
  query: string,
  field: string,
  value: string,
  mode: FilterMode
): string | null {
  if (isSearchFamily(type)) return searchFilter(query, field, value, mode)
  if (type === "loki") return lokiFilter(query, field, value, mode)
  if (type === "clickhouse") return clickhouseFilter(query, field, value, mode)
  return null
}
