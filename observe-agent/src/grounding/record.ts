import {
  compareBytes,
  byteLength,
  formatFloat,
  isSpace,
  parseFloat,
  trimSpace,
} from "../brief/gofmt"
import { valuesIn } from "./figures"
import { type Node, parseFirst } from "./node"
import { minute, parseTime } from "./time"

export type Mode = "off" | "enforce"

export const maxFacts = 400
export const maxValues = 2000
export const maxValuesOfRead = 500
export const maxNameBytes = 64
export const maxScannedBytes = 4 << 10
export const maxSourced = 2000
export const maxSourcedOfRead = 500
export const minNameRunes = 3

const maxNames = 400
const maxDiscovered = 1000
const maxRanges = 64
const maxInstants = 256
const maxIDs = 1000
const sigFigs = 3

export type FactKind = "number" | "duration" | "count" | "ratio" | "trace_id"

export type Fact = {
  id: string
  text: string
  value?: number
  kind: FactKind
  unit?: string
  of: string
  labels?: Record<string, string>
  at?: string
  from?: string
  to?: string
  query?: string
  asked?: string
  sampled?: boolean
  signal?: string
}

export type Range = { from: number; to: number }

export type GroundingRecord = {
  mode: Mode
  reads: number[][]
  names: string[]
  discovered: string[]
  sourced: string[]
  ranges: Range[]
  instants: number[]
  ids: string[]
  facts: Fact[]
  nextFact: number
}

export function newRecord(mode: Mode): GroundingRecord | undefined {
  if (mode !== "enforce") {
    return undefined
  }
  return {
    mode,
    reads: [],
    names: [],
    discovered: [],
    sourced: [],
    ranges: [],
    instants: [],
    ids: [],
    facts: [],
    nextFact: 0,
  }
}

export const quotedValue = /=[\t\n\f\r ]*"([^"]{1,64})"/gu
export const traceID = /^(?:[0-9a-fA-F]{32}|[0-9a-fA-F]{16})$/

export function rounded(v: number): number {
  v = Math.abs(v)
  if (v === 0 || !Number.isFinite(v)) {
    return v
  }
  return Number(formatFloat(v, "g", sigFigs))
}

export function normalName(s: string): string {
  let out = ""
  for (const ch of s) {
    if (/^[\p{L}\p{Nd}]$/u.test(ch)) {
      const lower = ch.toLowerCase()
      out +=
        [...lower].length === 1
          ? lower
          : String.fromCodePoint(lower.codePointAt(0) ?? 0)
    }
  }
  return out
}

const nameHolders = new Set(["labels", "attributes", "resource"])

const nameKeys = new Set([
  "service",
  "job",
  "pod",
  "podname",
  "namespace",
  "container",
  "containername",
  "deployment",
  "host",
  "hostname",
  "node",
  "nodename",
  "instance",
  "app",
  "component",
])

function nameKey(key: string): boolean {
  const k = normalName(key)
  return nameKeys.has(k) || k.endsWith("servicename")
}

const timeKeys = new Set([
  "time",
  "timens",
  "timestamp",
  "ts",
  "t",
  "start",
  "end",
  "from",
  "to",
  "at",
  "startns",
  "endns",
  "starttime",
  "endtime",
  "timeunixnano",
  "startunixnano",
  "endunixnano",
])

function timeKey(key: string): boolean {
  return timeKeys.has(normalName(key))
}

type Slot = { key: string; time: boolean; name: boolean }

const noSlot: Slot = { key: "", time: false, name: false }

function childOf(parent: string, key: string): Slot {
  const p = normalName(parent)
  return {
    key,
    time: timeKey(key),
    name:
      nameHolders.has(p) ||
      nameKey(key) ||
      (key === "name" && p === "services"),
  }
}

type Value =
  | { kind: "object"; map: Map<string, Value> }
  | { kind: "array"; vals: Value[] }
  | { kind: "string"; text: string }
  | { kind: "number"; text: string }
  | { kind: "literal" }

function valueOf(n: Node): Value {
  switch (n.kind) {
    case "object": {
      const map = new Map<string, Value>()
      n.keys.forEach((key, i) => map.set(key, valueOf(n.vals[i] as Node)))
      return { kind: "object", map }
    }
    case "array":
      return { kind: "array", vals: n.vals.map(valueOf) }
    case "string":
      return { kind: "string", text: n.text }
    case "number":
      return { kind: "number", text: n.text }
  }
  return { kind: "literal" }
}

class Reading {
  readonly seen = new Set<number>()
  readonly values: number[] = []

  constructor(readonly record: GroundingRecord) {}

  walk(v: Value, at: Slot): void {
    switch (v.kind) {
      case "object": {
        if (this.table(v.map)) {
          return
        }
        for (const key of [...v.map.keys()].sort(compareBytes)) {
          this.walk(v.map.get(key) as Value, childOf(at.key, key))
        }
        return
      }
      case "array":
        for (const child of v.vals) {
          this.walk(child, at)
        }
        return
      case "number": {
        const f = parseFloat(v.text)
        if (f !== undefined && !at.time) {
          this.value(f)
        }
        return
      }
      case "string":
        this.text(v.text, at)
    }
  }

  table(m: Map<string, Value>): boolean {
    const columns = m.get("columns")
    const rows = m.get("rows")
    if (columns?.kind !== "array" || rows?.kind !== "array") {
      return false
    }
    const cells: Slot[] = columns.vals.map((column) => {
      if (column.kind === "object") {
        const name = column.map.get("name")
        const kind = column.map.get("type")
        const n = name?.kind === "string" ? name.text : ""
        const k = kind?.kind === "string" ? kind.text : ""
        return { key: n, time: k === "time" || timeKey(n), name: nameKey(n) }
      }
      if (column.kind === "string") {
        return {
          key: column.text,
          time: timeKey(column.text),
          name: nameKey(column.text),
        }
      }
      return noSlot
    })
    for (const row of rows.vals) {
      if (row.kind !== "array") {
        continue
      }
      row.vals.forEach((cell, i) => {
        const slot = cells[i]
        if (slot) {
          this.walk(cell, slot)
        }
      })
    }
    return true
  }

  text(s: string, at: Slot): void {
    s = cut(s)
    const f = parseFloat(trimSpace(s))
    if (f !== undefined) {
      if (!at.time) {
        this.value(f)
      }
      return
    }
    if (traceID.test(s)) {
      addID(this.record, s)
      return
    }
    const t = parseTime(trimSpace(s))
    if (t !== undefined) {
      addInstant(this.record, t)
      return
    }
    if (at.name) {
      addName(this.record, s)
    }
    for (const m of s.matchAll(quotedValue)) {
      addName(this.record, m[1] ?? "")
    }
    for (const v of valuesIn(s)) {
      this.value(v)
    }
  }

  value(v: number): void {
    v = rounded(v)
    if (
      !Number.isFinite(v) ||
      this.seen.has(v) ||
      this.values.length >= maxValuesOfRead
    ) {
      return
    }
    this.seen.add(v)
    this.values.push(v)
  }
}

function cut(s: string): string {
  if (byteLength(s) <= maxScannedBytes) {
    return s
  }
  const raw = Buffer.from(s, "utf8").subarray(0, maxScannedBytes)
  let text = raw.toString("utf8")
  const chars = [...text]
  for (let i = chars.length - 1; i > 0; i--) {
    if (isSpace(chars[i] as string)) {
      return chars.slice(0, i).join("")
    }
  }
  return text
}

export function addRead(
  r: GroundingRecord | undefined,
  content: string,
  names: readonly string[],
  ranges: readonly Range[]
): void {
  if (!r) {
    return
  }
  const read = new Reading(r)
  const parsed = parseFirst(content)
  if (parsed) {
    read.walk(valueOf(parsed), noSlot)
  }
  if (read.values.length > 0) {
    r.reads.push(read.values)
    keepValues(r)
  }
  for (const name of names) {
    addDiscovered(r, name)
  }
  for (const rng of ranges) {
    addRange(r, rng)
  }
}

function keepValues(r: GroundingRecord): void {
  let over = r.reads.reduce((n, read) => n + read.length, 0) - maxValues
  while (over > 0) {
    const oldest = (r.reads[0] as number[]).length
    if (oldest <= over) {
      r.reads.shift()
      over -= oldest
      continue
    }
    r.reads[0] = (r.reads[0] as number[]).slice(over)
    over = 0
  }
}

function moveToNewest(list: string[], name: string, cap: number): void {
  const i = list.indexOf(name)
  if (i >= 0) {
    list.splice(i, 1)
  }
  list.push(name)
  if (list.length > cap) {
    list.splice(0, list.length - cap)
  }
}

export function addName(r: GroundingRecord, name: string): void {
  name = trimSpace(name)
  if (name === "" || byteLength(name) > maxNameBytes) {
    return
  }
  moveToNewest(r.names, name, maxNames)
}

function addDiscovered(r: GroundingRecord, name: string): void {
  name = trimSpace(name)
  if (
    name === "" ||
    byteLength(name) > maxNameBytes ||
    r.discovered.length >= maxDiscovered ||
    r.discovered.includes(name)
  ) {
    return
  }
  r.discovered.push(name)
}

function addRange(r: GroundingRecord, rng: Range): void {
  if (!(rng.from < rng.to)) {
    return
  }
  if (r.ranges.some((x) => x.from === rng.from && x.to === rng.to)) {
    return
  }
  r.ranges.push({ from: rng.from, to: rng.to })
  if (r.ranges.length > maxRanges) {
    r.ranges.splice(0, r.ranges.length - maxRanges)
  }
}

function addInstant(r: GroundingRecord, ms: number): void {
  const t = Math.floor(ms / 1000) * 1000
  if (r.instants.includes(t)) {
    return
  }
  r.instants.push(t)
  if (r.instants.length > maxInstants) {
    r.instants.splice(0, r.instants.length - maxInstants)
  }
}

function addID(r: GroundingRecord, id: string): void {
  id = id.toLowerCase()
  if (r.ids.includes(id)) {
    return
  }
  r.ids.push(id)
  if (r.ids.length > maxIDs) {
    r.ids.splice(0, r.ids.length - maxIDs)
  }
}

export function queried(r: GroundingRecord, at: number): boolean {
  for (const rng of r.ranges) {
    if (at >= rng.from - minute && at <= rng.to + minute) {
      return true
    }
  }
  return r.instants.some((seen) => Math.abs(at - seen) <= minute)
}

const sourceName = /(?:^|[^A-Za-z0-9])([A-Za-z][A-Za-z0-9_.-]*[A-Za-z0-9])/gu

export function sourceNames(text: string): string[] {
  const out: string[] = []
  for (const m of text.matchAll(sourceName)) {
    const name = m[1] as string
    if (
      name.length < minNameRunes ||
      name.length > maxNameBytes ||
      out.includes(name)
    ) {
      continue
    }
    out.push(name)
    if (out.length === maxSourcedOfRead) {
      break
    }
  }
  return out
}

export function addSourced(
  r: GroundingRecord | undefined,
  content: string
): void {
  if (!r) {
    return
  }
  let text: unknown
  try {
    text = (JSON.parse(content) as { text?: unknown } | null)?.text
  } catch {
    return
  }
  if (text !== undefined && text !== null && typeof text !== "string") {
    return
  }
  for (const name of sourceNames(text ?? "")) {
    const i = r.sourced.indexOf(name)
    if (i >= 0) {
      r.sourced.splice(i, 1)
    }
    r.sourced.push(name)
  }
  if (r.sourced.length > maxSourced) {
    r.sourced.splice(0, r.sourced.length - maxSourced)
  }
}

function sameLabels(
  a: Record<string, string> | undefined,
  b: Record<string, string> | undefined
): boolean {
  const ak = Object.keys(a ?? {})
  const bk = Object.keys(b ?? {})
  return (
    ak.length === bk.length &&
    ak.every((k) => b?.[k] === a?.[k] && k in (b ?? {}))
  )
}

function same(f: Fact, g: Fact): boolean {
  return (
    f.of === g.of &&
    f.text === g.text &&
    (f.at ?? "") === (g.at ?? "") &&
    (f.from ?? "") === (g.from ?? "") &&
    (f.to ?? "") === (g.to ?? "") &&
    (f.query ?? "") === (g.query ?? "") &&
    (f.asked ?? "") === (g.asked ?? "") &&
    (f.sampled ?? false) === (g.sampled ?? false) &&
    (f.unit ?? "") === (g.unit ?? "") &&
    sameLabels(f.labels, g.labels)
  )
}

export function addFact(r: GroundingRecord, f: Omit<Fact, "id">): string {
  const found = r.facts.find((g) => same(g, f as Fact))
  if (found) {
    return found.id
  }
  r.nextFact++
  const id = "f" + r.nextFact
  r.facts.push({ id, ...f })
  if (r.facts.length > maxFacts) {
    r.facts.splice(0, r.facts.length - maxFacts)
  }
  return id
}

export function factOf(
  r: GroundingRecord | undefined,
  id: string
): Fact | undefined {
  return r?.facts.find((f) => f.id === id)
}
