import type { Point } from "./types"

export type Rule = { factor: number; floor: number; sustain: number }

export const errorRule: Rule = { factor: 2, floor: 0.01, sustain: 3 }
export const latencyRule: Rule = { factor: 1.5, floor: 5, sustain: 3 }
export const logRule: Rule = { factor: 2, floor: 0.01, sustain: 3 }

const minTrailingRun = 2

export function threshold(r: Rule, baseline: number): number {
  return Math.max(r.factor * baseline, baseline + r.floor)
}

export function split(
  points: readonly Point[],
  windowStart: number
): { baseline: Point[]; window: Point[] } {
  const baseline: Point[] = []
  const window: Point[] = []
  for (const p of points) {
    if (!Number.isFinite(p.value)) {
      continue
    }
    if (p.at < windowStart) {
      baseline.push(p)
    } else {
      window.push(p)
    }
  }
  return { baseline, window }
}

export function values(points: readonly Point[]): number[] {
  return points.map((p) => p.value)
}

export function median(v: readonly number[]): number {
  if (v.length === 0) {
    return 0
  }
  const s = [...v].sort((a, b) => a - b)
  const mid = s.length >> 1
  return s.length % 2 === 1
    ? (s[mid] as number)
    : ((s[mid - 1] as number) + (s[mid] as number)) / 2
}

export function onset(
  points: readonly Point[],
  windowStart: number,
  r: Rule
): number | undefined {
  const { baseline, window } = split(points, windowStart)
  if (window.length === 0) {
    return undefined
  }
  const limit = threshold(r, median(values(baseline)))
  const sustain = Math.max(r.sustain, 1)
  let run = 0
  for (let i = 0; i < window.length; i++) {
    if (!((window[i] as Point).value > limit)) {
      run = 0
      continue
    }
    run++
    if (run === sustain) {
      return (window[i - run + 1] as Point).at
    }
  }
  if (run >= Math.min(sustain, minTrailingRun)) {
    return (window[window.length - run] as Point).at
  }
  return undefined
}
