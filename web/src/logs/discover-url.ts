import { isTimeValue } from "@/lib/time"

import type { NewPill } from "./pills"
import { withForm } from "./query-form"
import { decodeRison, RisonError } from "./rison"

export class DiscoverUrlError extends Error {
  constructor(message: string) {
    super(message)
    this.name = "DiscoverUrlError"
  }
}

export type DiscoverImport = {
  q?: string
  from?: string
  to?: string
  cols?: string[]
  pills: NewPill[]
  dropped: number
}

type JsonObject = Record<string, unknown>

const discoverPath = /\/app\/(?:data-explorer\/)?discover#[^?\s]*\?(\S*)$/

export function isDiscoverUrl(text: string): boolean {
  return discoverPath.test(text.trim())
}

function recordOf(value: unknown): JsonObject | undefined {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as JsonObject)
    : undefined
}

function stateOf(params: URLSearchParams, name: string): JsonObject {
  const text = params.get(name)
  if (text === null) return {}
  try {
    return recordOf(decodeRison(text)) ?? {}
  } catch (error) {
    if (error instanceof RisonError) {
      throw new DiscoverUrlError(`${name}: ${error.message}`)
    }
    throw error
  }
}

function scalarText(value: unknown): string | undefined {
  if (typeof value === "string") return value
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value)
  }
  return undefined
}

function queryOf(state: JsonObject): string | undefined {
  const query = recordOf(state.query)
  const text = query?.query
  if (typeof text !== "string" || text.trim() === "") return undefined
  return query?.language === "lucene" ? withForm(text, "lucene") : text
}

function columnsOf(state: JsonObject): string[] | undefined {
  const columns = state.columns ?? recordOf(state.discover)?.columns
  if (!Array.isArray(columns)) return undefined
  const names = columns.filter(
    (name): name is string =>
      typeof name === "string" &&
      name !== "" &&
      name !== "_source" &&
      !name.includes(",")
  )
  return names.length === 0 ? undefined : names
}

function rangeOf(state: JsonObject): { from: string; to: string } | undefined {
  const time = recordOf(state.time)
  const from = scalarText(time?.from)
  const to = scalarText(time?.to)
  return from !== undefined &&
    to !== undefined &&
    isTimeValue(from) &&
    isTimeValue(to)
    ? { from, to }
    : undefined
}

function pillOf(filter: unknown): NewPill | null {
  const entry = recordOf(filter)
  const meta = recordOf(entry?.meta)
  const field = meta?.key
  if (typeof field !== "string" || field === "") return null
  const base = {
    field,
    negate: meta?.negate === true,
    pinned: false,
    paused: meta?.disabled === true,
  }
  const params = meta?.params
  if (meta?.type === "phrase") {
    const value = scalarText(recordOf(params)?.query ?? params)
    return value === undefined ? null : { ...base, kind: "is", value }
  }
  if (meta?.type === "phrases" && Array.isArray(params)) {
    const values = params.map(scalarText)
    return values.length > 0 && values.every((value) => value !== undefined)
      ? { ...base, kind: "oneOf", values: values as string[] }
      : null
  }
  return null
}

export function importDiscoverUrl(text: string): DiscoverImport {
  const matched = discoverPath.exec(text.trim())
  if (matched === null) throw new DiscoverUrlError("not a Discover link")
  const params = new URLSearchParams(matched[1])
  const global = stateOf(params, "_g")
  const app = stateOf(params, "_a")
  const query = stateOf(params, "_q")
  const filters = [global, app, query].flatMap((state) =>
    Array.isArray(state.filters) ? state.filters : []
  )
  const pills: NewPill[] = []
  let dropped = 0
  for (const filter of filters) {
    const pill = pillOf(filter)
    if (pill === null) dropped++
    else pills.push(pill)
  }
  return {
    q: queryOf(app) ?? queryOf(query),
    ...rangeOf(global),
    cols: columnsOf(app),
    pills,
    dropped,
  }
}

function counted(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? "" : "s"}`
}

export function importSummary(result: DiscoverImport): string {
  const filled = [
    result.q === undefined ? null : "query",
    result.from === undefined ? null : "time range",
    result.cols === undefined ? null : counted(result.cols.length, "column"),
    result.pills.length === 0 ? null : counted(result.pills.length, "filter"),
  ].filter((part) => part !== null)
  const head =
    filled.length === 0
      ? "Nothing in that Discover link could be used."
      : `Filled from a Discover link: ${filled.join(", ")}.`
  if (result.dropped === 0) return head
  const noun = result.dropped === 1 ? "filter" : "filters"
  return `${head} ${result.dropped} ${noun} from the URL ${result.dropped === 1 ? "was" : "were"} not applied.`
}
