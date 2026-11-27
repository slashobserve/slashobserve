import { compareBytes } from "../brief/gofmt"
import {
  type Candidate,
  type Event,
  type Findings,
  minute,
  type Origin,
  type Pattern,
  type Row,
  signalErrors,
  signalLatency,
} from "./types"

export const eventLookback = 30 * minute
export const eventSlack = 5 * minute

const maxCandidateOrigins = 3
const maxCandidateEvents = 5
const maxCandidatePattern = 3

export type Move = { service: string; rows: Row[]; onset?: number | undefined }

const cmp = (a: number, b: number) => (a < b ? -1 : a > b ? 1 : 0)

export function movesOf(rows: readonly Row[]): Move[] {
  const byService = new Map<string, Move>()
  for (const r of rows) {
    if (!r.moved || (r.signal !== signalErrors && r.signal !== signalLatency)) {
      continue
    }
    let m = byService.get(r.service)
    if (!m) {
      m = { service: r.service, rows: [] }
      byService.set(r.service, m)
    }
    keepFinest(m, r)
  }
  const moves = [...byService.values()].map((m) => {
    m.rows.sort((a, b) => compareBytes(a.signal, b.signal))
    for (const r of m.rows) {
      if (
        r.onset !== undefined &&
        (m.onset === undefined || r.onset < m.onset)
      ) {
        m.onset = r.onset
      }
    }
    return m
  })
  return moves.sort(byOnset)
}

function keepFinest(m: Move, r: Row): void {
  const i = m.rows.findIndex((x) => x.signal === r.signal)
  if (i >= 0) {
    if ((m.rows[i] as Row).rung <= r.rung) {
      return
    }
    m.rows.splice(i, 1)
  }
  m.rows.push(r)
}

function byOnset(a: Move, b: Move): number {
  if (a.onset === undefined && b.onset === undefined) {
    return compareBytes(a.service, b.service)
  }
  if (a.onset === undefined) {
    return 1
  }
  if (b.onset === undefined) {
    return -1
  }
  return cmp(a.onset, b.onset) || compareBytes(a.service, b.service)
}

function upstreamOf(
  m: Move,
  candidates: readonly Candidate[],
  root: ReadonlyMap<string, number>,
  origins: readonly Origin[]
): number | undefined {
  for (const o of origins) {
    if (o.service === m.service || !o.relayedBy.includes(m.service)) {
      continue
    }
    const i = root.get(o.service)
    if (i === undefined) {
      continue
    }
    const up = candidates[i] as Candidate
    if (m.onset !== undefined && up.onset !== undefined && m.onset < up.onset) {
      continue
    }
    return i
  }
  return undefined
}

function originsOf(service: string, origins: readonly Origin[]): Origin[] {
  return origins
    .filter((o) => o.service === service)
    .sort((a, b) => cmp(b.traces, a.traces))
    .slice(0, maxCandidateOrigins)
}

function eventsNear(c: Candidate, events: readonly Event[]): Event[] {
  if (c.onset === undefined) {
    return []
  }
  const from = c.onset - eventLookback
  const to = c.onset + eventSlack
  return events
    .filter(
      (e) =>
        e.at >= from &&
        e.at <= to &&
        (e.service === "" ||
          e.service === c.service ||
          c.knockOn.includes(e.service))
    )
    .sort((a, b) => cmp(b.at, a.at))
    .slice(0, maxCandidateEvents)
}

function patternsOf(service: string, patterns: readonly Pattern[]): Pattern[] {
  return patterns
    .filter((p) => p.service === service)
    .sort((a, b) => cmp(b.window - b.baseline, a.window - a.baseline))
    .slice(0, maxCandidatePattern)
}

function backgroundOf(
  rows: readonly Row[],
  candidates: ReadonlyMap<string, number>
): Row[] {
  const out: Row[] = []
  for (const r of rows) {
    if (
      r.moved ||
      r.signal !== signalErrors ||
      r.window <= 0 ||
      r.baseline <= 0
    ) {
      continue
    }
    if (candidates.has(r.service)) {
      continue
    }
    const i = out.findIndex((x) => x.service === r.service)
    if (i >= 0) {
      if ((out[i] as Row).rung > r.rung) {
        out[i] = r
      }
      continue
    }
    out.push(r)
  }
  return out.sort((a, b) => compareBytes(a.service, b.service))
}

export function cluster(f: Findings): {
  candidates: Candidate[]
  background: Row[]
} {
  const candidates: Candidate[] = []
  const root = new Map<string, number>()
  for (const m of movesOf(f.rows)) {
    const i = upstreamOf(m, candidates, root, f.origins)
    if (i !== undefined) {
      ;(candidates[i] as Candidate).knockOn.push(m.service)
      root.set(m.service, i)
      continue
    }
    root.set(m.service, candidates.length)
    candidates.push({
      id: "",
      service: m.service,
      onset: m.onset,
      rows: m.rows,
      knockOn: [],
      origins: [],
      events: [],
      patterns: [],
    })
  }
  candidates.forEach((c, i) => {
    c.id = `c${i + 1}`
    c.origins = originsOf(c.service, f.origins)
    c.events = eventsNear(c, f.events)
    c.patterns = patternsOf(c.service, f.patterns)
  })
  return { candidates, background: backgroundOf(f.rows, root) }
}
