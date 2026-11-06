import { perLineLokiLabels } from "./context-query"
import { appendExists, appendIn, appendKql, appendRange } from "./kql"
import { filterTarget, withFilter } from "./query-filters"
import { splitForm } from "./query-form"
import { isSearchFamily } from "./search"

export type PillKind = "is" | "oneOf" | "range" | "exists"
type RangeOperator = ">" | ">=" | "<" | "<="

export type Pill = {
  id: string
  kind: PillKind
  field: string
  value?: string
  values?: string[]
  op?: RangeOperator
  negate: boolean
  pinned: boolean
  paused: boolean
}

export type NewPill = Omit<Pill, "id">

export const maxPills = 50

const maxFieldLength = 256
const kinds: readonly PillKind[] = ["is", "oneOf", "range", "exists"]
const operators: readonly RangeOperator[] = [">", ">=", "<", "<="]
const negateFlag = 1
const pinnedFlag = 2
const pausedFlag = 4
const allFlags = negateFlag | pinnedFlag | pausedFlag

type Entry = [
  PillKind,
  string,
  string | string[] | null,
  number,
  RangeOperator?,
]

function toBase64Url(text: string): string {
  const binary = String.fromCharCode(...new TextEncoder().encode(text))
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "")
}

function fromBase64Url(text: string): string {
  const base64 = text.replace(/-/g, "+").replace(/_/g, "/")
  const binary = atob(base64.padEnd(Math.ceil(base64.length / 4) * 4, "="))
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0))
  return new TextDecoder("utf-8", { fatal: true }).decode(bytes)
}

function flagsOf(pill: NewPill): number {
  return (
    (pill.negate ? negateFlag : 0) |
    (pill.pinned ? pinnedFlag : 0) |
    (pill.paused ? pausedFlag : 0)
  )
}

function entryOf(pill: NewPill): Entry {
  const flags = flagsOf(pill)
  switch (pill.kind) {
    case "is":
      return ["is", pill.field, pill.value ?? "", flags]
    case "oneOf":
      return ["oneOf", pill.field, pill.values ?? [], flags]
    case "range":
      return ["range", pill.field, pill.value ?? "", flags, pill.op ?? ">="]
    case "exists":
      return ["exists", pill.field, null, flags]
  }
}

export function encodePills(pills: Pill[]): string | undefined {
  if (pills.length === 0) return undefined
  return toBase64Url(JSON.stringify(pills.slice(0, maxPills).map(entryOf)))
}

function isStringList(value: unknown): value is string[] {
  return (
    Array.isArray(value) &&
    value.length > 0 &&
    value.every((item) => typeof item === "string")
  )
}

function pillOfEntry(entry: unknown, index: number): Pill | null {
  if (!Array.isArray(entry)) return null
  const [kind, field, payload, flags, op] = entry as unknown[]
  if (!kinds.includes(kind as PillKind)) return null
  if (
    typeof field !== "string" ||
    field === "" ||
    field.length > maxFieldLength
  ) {
    return null
  }
  if (
    typeof flags !== "number" ||
    !Number.isInteger(flags) ||
    flags < 0 ||
    flags > allFlags
  ) {
    return null
  }
  const base = {
    id: String(index),
    kind: kind as PillKind,
    field,
    negate: (flags & negateFlag) !== 0,
    pinned: (flags & pinnedFlag) !== 0,
    paused: (flags & pausedFlag) !== 0,
  }
  switch (base.kind) {
    case "is":
      return typeof payload === "string" ? { ...base, value: payload } : null
    case "oneOf":
      return isStringList(payload) ? { ...base, values: payload } : null
    case "range":
      return typeof payload === "string" &&
        operators.includes(op as RangeOperator)
        ? { ...base, negate: false, value: payload, op: op as RangeOperator }
        : null
    case "exists":
      return base
  }
}

export function decodePills(text: string | undefined): Pill[] {
  if (text === undefined || text === "") return []
  let parsed: unknown
  try {
    parsed = JSON.parse(fromBase64Url(text))
  } catch {
    return []
  }
  if (!Array.isArray(parsed)) return []
  const pills: Pill[] = []
  for (const entry of parsed) {
    const pill = pillOfEntry(entry, pills.length)
    if (pill !== null) pills.push(pill)
    if (pills.length === maxPills) break
  }
  return pills
}

export function pillLabel(pill: Pill): string {
  const prefix = pill.negate ? "NOT " : ""
  switch (pill.kind) {
    case "is":
      return `${prefix}${pill.field}: ${pill.value ?? ""}`
    case "oneOf":
      return `${prefix}${pill.field}: ${(pill.values ?? []).join(", ")}`
    case "range":
      return `${pill.field} ${pill.op ?? ">="} ${pill.value ?? ""}`
    case "exists":
      return `${prefix}${pill.field}: exists`
  }
}

function sameClause(a: NewPill, b: NewPill): boolean {
  return (
    a.kind === b.kind &&
    a.field === b.field &&
    a.negate === b.negate &&
    a.op === b.op &&
    a.value === b.value &&
    (a.values ?? []).join("\0") === (b.values ?? []).join("\0")
  )
}

function renumbered(pills: Pill[]): Pill[] {
  return pills.map((pill, index) => ({ ...pill, id: String(index) }))
}

export function addPill(pills: Pill[], next: NewPill): Pill[] {
  const twin = pills.findIndex((pill) => sameClause(pill, next))
  if (twin >= 0) {
    if (!pills[twin].paused) return pills
    return pills.map((pill, index) =>
      index === twin ? { ...pill, paused: false } : pill
    )
  }
  if (pills.length >= maxPills) return pills
  return renumbered([...pills, { ...next, id: "" }])
}

export function replacePill(
  pills: Pill[],
  id: string,
  next: NewPill[]
): Pill[] {
  const index = pills.findIndex((pill) => pill.id === id)
  if (index < 0) return pills
  const rest = [...pills.slice(0, index), ...pills.slice(index + 1)]
  const merged = next.reduce((current, pill) => addPill(current, pill), rest)
  return renumbered(merged)
}

export function removePill(pills: Pill[], id: string): Pill[] {
  return renumbered(pills.filter((pill) => pill.id !== id))
}

export function pillSupported(type: string, pill: Pill): boolean {
  if (isSearchFamily(type)) return true
  return (type === "loki" || type === "clickhouse") && pill.kind === "is"
}

function applySearchPill(query: string, pill: Pill): string {
  switch (pill.kind) {
    case "is":
      return appendKql(query, pill.field, pill.value ?? "", pill.negate)
    case "oneOf":
      return appendIn(query, pill.field, pill.values ?? [], pill.negate)
    case "exists":
      return appendExists(query, pill.field, pill.negate)
    case "range":
      return appendRange(query, pill.field, pill.op ?? ">=", pill.value ?? "")
  }
}

function applyNativePill(
  query: string,
  type: "loki" | "clickhouse",
  pill: Pill
): string | null {
  if (pill.kind !== "is") return null
  const stream = type === "loki" && !perLineLokiLabels.has(pill.field)
  const target = filterTarget(type, pill.field, stream)
  return withFilter(
    query,
    type,
    target,
    pill.field,
    pill.value ?? "",
    pill.negate
  )
}

export function applyPills(
  query: string,
  type: string,
  pills: Pill[]
): string | null {
  const active = pills.filter((pill) => !pill.paused)
  if (active.length === 0) return query
  if (isSearchFamily(type)) {
    const { form, body, offset } = splitForm(query)
    if (form !== "kql") return null
    const applied = active.reduce(
      (text, pill) => applySearchPill(text, pill),
      body
    )
    return query.slice(0, offset) + applied
  }
  if (type !== "loki" && type !== "clickhouse") return null
  return active.reduce(
    (text, pill) => applyNativePill(text, type, pill) ?? text,
    query
  )
}

export function unpinnedCleared(pills: Pill[]): Pill[] {
  return renumbered(pills.filter((pill) => pill.pinned))
}

export function pillOptions(type: string): {
  kinds: PillKind[]
  negate: boolean
} {
  if (isSearchFamily(type)) {
    return { kinds: ["is", "oneOf", "range", "exists"], negate: true }
  }
  return { kinds: ["is"], negate: true }
}
