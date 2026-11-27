import { limitGrowth } from "./text"
import type { Hypothesis, Stage } from "./types"

export const statusOpen = "open"
export const statusSupported = "supported"

export type Loop = {
  limit: number
  max: number
  base: Stage | ""
  readSinceGrowth: boolean
  quietTurns: number
  reminded: boolean
  concludeSent: boolean
  shown: Stage | ""
  savedStage: Stage | ""
  candidates: string[]
}

export function startLoop(limit: number, callsMax: number): Loop {
  return {
    limit,
    max: Math.max(callsMax, limit),
    base: "",
    readSinceGrowth: false,
    quietTurns: 0,
    reminded: false,
    concludeSent: false,
    shown: "",
    savedStage: "",
    candidates: [],
  }
}

function sameLines(a: readonly string[] = [], b: readonly string[] = []) {
  return a.length === b.length && a.every((line, i) => line === b[i])
}

export function sameLedger(
  a: readonly Hypothesis[],
  b: readonly Hypothesis[]
): boolean {
  return (
    a.length === b.length &&
    a.every((x, i) => {
      const y = b[i] as Hypothesis
      return (
        x.id === y.id &&
        x.text === y.text &&
        x.status === y.status &&
        (x.closing ?? "") === (y.closing ?? "") &&
        (x.knock_on_of ?? "") === (y.knock_on_of ?? "") &&
        (x.next ?? "") === (y.next ?? "") &&
        sameLines(x.evidence, y.evidence)
      )
    })
  )
}

export function baseStage(ledger: readonly Hypothesis[]): Stage {
  if (ledger.length === 0) {
    return "orient"
  }
  const evidence = ledger.some((h) => (h.evidence ?? []).length > 0)
  const supported = ledger.some((h) => h.status === statusSupported)
  const open = ledger.some((h) => h.status === statusOpen)
  if (!evidence) {
    return "hypothesize"
  }
  return supported && !open ? "conclude" : "test"
}

export function provedSomething(
  prev: readonly Hypothesis[],
  next: readonly Hypothesis[]
): boolean {
  const known = new Set(prev.flatMap((p) => p.evidence ?? []))
  const newLine = (h: Hypothesis) =>
    (h.evidence ?? []).some((line) => !known.has(line))
  return next.some((h) => {
    if (h.status === statusOpen) {
      return false
    }
    const before = prev.find((p) => p.id === h.id)
    if (before === undefined) {
      return newLine(h)
    }
    return (
      before.status === statusOpen || (before.status !== h.status && newLine(h))
    )
  })
}

function closedCandidates(
  candidates: readonly string[],
  prev: readonly Hypothesis[],
  next: readonly Hypothesis[]
): number {
  const closed = (ledger: readonly Hypothesis[], id: string) =>
    ledger.some(
      (h) =>
        h.id === id &&
        h.status !== statusOpen &&
        (h.closing ?? "") !== "" &&
        (h.evidence ?? []).length > 0
    )
  return candidates.filter((id) => closed(next, id) && !closed(prev, id)).length
}

export function noteLedger(
  loop: Loop,
  prev: readonly Hypothesis[],
  next: readonly Hypothesis[]
): boolean {
  loop.base = baseStage(next)
  if (sameLedger(prev, next)) {
    return false
  }
  if (loop.readSinceGrowth && provedSomething(prev, next)) {
    const step =
      limitGrowth * Math.max(1, closedCandidates(loop.candidates, prev, next))
    const grown = Math.min(loop.limit + step, loop.max)
    if (grown > loop.limit) {
      loop.limit = grown
      loop.readSinceGrowth = false
    }
  }
  return true
}

export function stageOf(loop: Loop, calls: number): Stage {
  if (loop.base === "conclude" || calls >= loop.limit - 1) {
    return "conclude"
  }
  return loop.base === "" ? "orient" : loop.base
}
