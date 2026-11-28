import { compareBytes, formatFloat } from "../brief/gofmt"
import { logFlagKeys } from "./queries"
import type { Series, TraceGroup } from "./rates"
import {
  type Event,
  eventDeployLine,
  eventFlag,
  minute,
  type Pattern,
  type Point,
} from "./types"

const maxBurstsPerService = 3
const minNewPattern = 3
const minGrownPattern = 10

const cmp = (a: number, b: number) => (a < b ? -1 : a > b ? 1 : 0)

function gapOf(points: readonly Point[]): number {
  let gap = 0
  for (let i = 1; i < points.length; i++) {
    const d = (points[i] as Point).at - (points[i - 1] as Point).at
    if (gap === 0 || d < gap) {
      gap = d
    }
  }
  return gap
}

export function burstStarts(
  series: readonly Series[],
  label: string,
  uid: string
): Event[] {
  const events: Event[] = []
  for (const s of series) {
    const name = s.labels[label] ?? ""
    if (name === "") {
      continue
    }
    const points = [...s.points].sort((a, b) => cmp(a.at, b.at))
    const gap = gapOf(points)
    const starts: Event[] = []
    points.forEach((p, i) => {
      const prev = points[i - 1]
      if (
        p.value <= 0 ||
        (prev !== undefined && prev.value > 0 && p.at - prev.at <= gap)
      ) {
        return
      }
      starts.push({
        kind: eventDeployLine,
        source: uid,
        service: name,
        at: p.at,
        text: `${formatFloat(p.value, "f", 0)} lines in the first step`,
      })
    })
    events.push(
      ...starts.slice(Math.max(0, starts.length - maxBurstsPerService))
    )
  }
  return events
}

export function flagEvent(
  uid: string,
  key: string,
  variant: string,
  at: number,
  from: number
): Event {
  const shown = variant === "" ? "(none)" : variant
  const text =
    at > from + minute
      ? `${key} variant ${shown} first seen`
      : `${key} variant ${shown} already evaluated when the read began`
  return { kind: eventFlag, source: uid, service: "", at, text }
}

export function flagFirstSeen(
  series: readonly Series[],
  uid: string,
  from: number
): Event[] {
  const events: Event[] = []
  for (const s of series) {
    const key = s.labels[logFlagKeys[0] as string] ?? ""
    const variant =
      (s.labels[logFlagKeys[1] as string] ?? "") ||
      (s.labels[logFlagKeys[2] as string] ?? "")
    if (key === "") {
      continue
    }
    const first = s.points.find((p) => p.value > 0)
    if (first) {
      events.push(flagEvent(uid, key, variant, first.at, from))
    }
  }
  return events.sort((a, b) => cmp(a.at, b.at) || compareBytes(a.text, b.text))
}

export function topGroups(groups: readonly TraceGroup[], n: number): string[] {
  return groups
    .filter((g) => g.value !== "" && g.spans !== 0)
    .sort((a, b) => cmp(b.spans, a.spans) || compareBytes(a.value, b.value))
    .slice(0, n)
    .map((g) => g.value)
}

export type PatternTrend = { text: string; level: string; trend: number[] }

export function changedPatterns(
  found: readonly PatternTrend[],
  uid: string,
  service: string
): Pattern[] {
  const out: Pattern[] = []
  for (const p of found) {
    const half = p.trend.length >> 1
    let before = 0
    let during = 0
    p.trend.forEach((n, i) => {
      if (i < half) {
        before += n
      } else {
        during += n
      }
    })
    const isNew = before === 0 && during >= minNewPattern
    const grew =
      before > 0 && during >= 2 * before && during - before >= minGrownPattern
    if (isNew || grew) {
      out.push({
        source: uid,
        service,
        level: p.level,
        text: p.text,
        window: during,
        baseline: before,
      })
    }
  }
  return out
}
