import { compareBytes, toLower } from "../brief/gofmt"
import {
  errorRule,
  latencyRule,
  logRule,
  median,
  onset,
  type Rule,
  split,
  threshold,
  values,
} from "./onset"
import { errorLevels, levelLabel } from "./queries"
import {
  type Point,
  type Row,
  rungs,
  signalErrors,
  signalLatency,
  signalRequests,
} from "./types"

export type Series = { labels: Record<string, string>; points: Point[] }

export type TraceGroup = {
  value: string
  spans: number
  errors: number
  p95ms?: number | undefined
}

export type TraceBucket = {
  start: number
  spans: number
  errors: number
  p95ms?: number | undefined
}

const maxRateRows = 10
const minBaselineObs = 3

const cmp = (a: number, b: number) => (a < b ? -1 : a > b ? 1 : 0)

export function byService(
  series: readonly Series[],
  label: string
): Map<string, Point[]> {
  const out = new Map<string, Point[]>()
  for (const s of series) {
    const name = s.labels[label] ?? ""
    if (name !== "") {
      out.set(name, [...(out.get(name) ?? []), ...s.points])
    }
  }
  for (const points of out.values()) {
    points.sort((a, b) => cmp(a.at, b.at))
  }
  return out
}

function sumOf(points: readonly Point[]): number {
  let s = 0
  for (const p of points) {
    s += p.value
  }
  return s
}

function meanOf(points: readonly Point[]): number {
  return points.length === 0 ? 0 : sumOf(points) / points.length
}

function share(errs: readonly Point[], req: readonly Point[]): number {
  const total = sumOf(req)
  return total <= 0 ? 0 : sumOf(errs) / total
}

export function ratioSeries(
  req: readonly Point[],
  errs: readonly Point[]
): Point[] {
  const errAt = new Map<number, number>()
  for (const p of errs) {
    errAt.set(p.at, p.value)
  }
  return req
    .filter((p) => p.value > 0)
    .map((p) => ({ at: p.at, value: (errAt.get(p.at) ?? 0) / p.value }))
}

function row(base: Row, service: string, signal: string, unit: string): Row {
  return { ...base, service, signal, unit }
}

function requestRow(
  base: Row,
  name: string,
  windowStart: number,
  points: readonly Point[]
): Row {
  const { baseline, window } = split(points, windowStart)
  return {
    ...row(base, name, signalRequests, "/s"),
    window: meanOf(window),
    baseline: meanOf(baseline),
  }
}

function errorRow(
  base: Row,
  name: string,
  windowStart: number,
  req: readonly Point[],
  errs: readonly Point[]
): Row | undefined {
  const ratios = ratioSeries(req, errs)
  if (ratios.length === 0) {
    return undefined
  }
  const r = split(req, windowStart)
  const e = split(errs, windowStart)
  const out: Row = {
    ...row(base, name, signalErrors, "ratio"),
    window: share(e.window, r.window),
    baseline: share(e.baseline, r.baseline),
  }
  out.moved =
    sumOf(e.window) > 0 && out.window > threshold(errorRule, out.baseline)
  if (out.moved) {
    out.onset = onset(ratios, windowStart, errorRule)
  }
  return out
}

function latencyRow(
  base: Row,
  name: string,
  windowStart: number,
  points: readonly Point[],
  scale: number
): Row | undefined {
  const scaled = points.map((p) => ({ at: p.at, value: p.value * scale }))
  const { baseline, window } = split(scaled, windowStart)
  if (window.length === 0) {
    return undefined
  }
  const out: Row = {
    ...row(base, name, signalLatency, "ms"),
    window: median(values(window)),
    baseline: median(values(baseline)),
  }
  out.moved =
    baseline.length >= minBaselineObs &&
    out.window > threshold(latencyRule, out.baseline)
  if (out.moved) {
    out.onset = onset(scaled, windowStart, latencyRule)
  }
  return out
}

export function metricRows(
  base: Row,
  windowStart: number,
  req: ReadonlyMap<string, Point[]>,
  errs: ReadonlyMap<string, Point[]>,
  lat: ReadonlyMap<string, Point[]>,
  scale: number,
  errsRead: boolean
): Row[] {
  const rows: Row[] = []
  for (const name of [...req.keys()].sort(compareBytes)) {
    const points = req.get(name) as Point[]
    rows.push(requestRow(base, name, windowStart, points))
    if (errsRead) {
      const e = errorRow(base, name, windowStart, points, errs.get(name) ?? [])
      if (e) {
        rows.push(e)
      }
    }
    const l = latencyRow(base, name, windowStart, lat.get(name) ?? [], scale)
    if (l) {
      rows.push(l)
    }
  }
  return rows
}

export function topRates(rows: readonly Row[]): Row[] {
  const req = rows
    .filter((r) => r.signal === signalRequests && r.rung === rungs[0])
    .sort(
      (a, b) => cmp(b.window, a.window) || compareBytes(a.service, b.service)
    )
    .slice(0, maxRateRows)
  const out: Row[] = []
  for (const r of req) {
    out.push(r)
    for (const other of rows) {
      if (
        other.service === r.service &&
        other.source === r.source &&
        other.rung === r.rung &&
        other.signal !== signalRequests
      ) {
        out.push(other)
      }
    }
  }
  return out
}

function ratio(n: number, d: number): number {
  return d === 0 ? 0 : n / d
}

export function traceRows(
  base: Row,
  now: readonly TraceGroup[],
  before: readonly TraceGroup[]
): Row[] {
  const prior = new Map<string, TraceGroup>()
  for (const g of before) {
    prior.set(g.value, g)
  }
  const seconds = base.rung / 1000
  const rows: Row[] = []
  for (const g of now) {
    if (g.value === "" || g.spans === 0) {
      continue
    }
    const b = prior.get(g.value) ?? { value: g.value, spans: 0, errors: 0 }
    const req: Row = {
      ...row(base, g.value, signalRequests, "spans/s"),
      window: g.spans / seconds,
      baseline: b.spans / seconds,
    }
    const errs: Row = {
      ...row(base, g.value, signalErrors, "ratio"),
      window: ratio(g.errors, g.spans),
      baseline: ratio(b.errors, b.spans),
    }
    errs.moved =
      g.errors > 0 && errs.window > threshold(errorRule, errs.baseline)
    rows.push(req, errs)
    if (g.p95ms !== undefined && b.p95ms !== undefined) {
      const lat: Row = {
        ...row(base, g.value, signalLatency, "ms"),
        window: g.p95ms,
        baseline: b.p95ms,
      }
      lat.moved =
        b.spans > 0 && lat.window > threshold(latencyRule, lat.baseline)
      rows.push(lat)
    }
  }
  return rows
}

function onsetOrNone(
  points: readonly Point[],
  windowStart: number,
  r: Rule
): number | undefined {
  return onset(points, windowStart, r)
}

export function bucketOnsets(
  buckets: readonly TraceBucket[],
  windowStart: number
): Record<string, number | undefined> {
  const ratios: Point[] = []
  const lat: Point[] = []
  for (const b of buckets) {
    if (b.spans > 0) {
      ratios.push({ at: b.start, value: ratio(b.errors, b.spans) })
    }
    if (b.p95ms !== undefined) {
      lat.push({ at: b.start, value: b.p95ms })
    }
  }
  return {
    [signalErrors]: onsetOrNone(ratios, windowStart, errorRule),
    [signalLatency]: onsetOrNone(lat, windowStart, latencyRule),
  }
}

export function errorLines(
  series: readonly Series[],
  label: string
): Map<string, Point[]> {
  const grid = new Map<number, number>()
  const sums = new Map<string, Map<number, number>>()
  for (const s of series) {
    const name = s.labels[label] ?? ""
    if (name === "") {
      continue
    }
    let at = sums.get(name)
    if (!at) {
      at = new Map()
      sums.set(name, at)
    }
    const isError = errorLevels.includes(toLower(s.labels[levelLabel] ?? ""))
    for (const p of s.points) {
      grid.set(p.at, p.at)
      if (isError) {
        at.set(p.at, (at.get(p.at) ?? 0) + p.value)
      }
    }
  }
  const steps = [...grid.keys()].sort(cmp)
  const out = new Map<string, Point[]>()
  for (const [name, at] of sums) {
    out.set(
      name,
      steps.map((k) => ({ at: k, value: at.get(k) ?? 0 }))
    )
  }
  return out
}

export function logRows(
  base: Row,
  windowStart: number,
  lines: ReadonlyMap<string, Point[]>,
  step: number
): Row[] {
  const rows: Row[] = []
  const seconds = base.rung / 1000
  for (const name of [...lines.keys()].sort(compareBytes)) {
    const points = lines.get(name) as Point[]
    const perSecond = points.map((p) => ({
      at: p.at,
      value: p.value / (step / 1000),
    }))
    const { baseline, window } = split(points, windowStart)
    const out: Row = {
      ...row(base, name, signalErrors, "error lines/s"),
      window: sumOf(window) / seconds,
      baseline: sumOf(baseline) / seconds,
    }
    out.moved = out.window > threshold(logRule, out.baseline)
    if (out.moved) {
      out.onset = onset(perSecond, windowStart, logRule)
    }
    rows.push(out)
  }
  return rows
}
