import { clipBytes, toLower, trimSpace } from "../brief/gofmt"
import { signalLogs, signalMetrics, signalTraces } from "./facts"
import { type CauseInput, parseCauses } from "./input"
import { itemAction, kinds, type Problem } from "./problems"
import { type Fact, type GroundingRecord, queried } from "./record"
import { citations, type Ledger } from "./render"
import { knownIDs, ledgerOf, sawName } from "./review"
import { parseTime } from "./time"

export const maxCauseFacts = 12
export const maxCauseQueries = 6
const maxQueryBytes = 400
const onsetAgreement = 5 * 60000

export type Signals = {
  kinds: string[]
  component: boolean
  onset_read: boolean
  onset_fact: boolean
  trigger_read: boolean
  ruled_out: boolean
}

export function citedFacts(...texts: string[]): string[] {
  const ids: string[] = []
  for (const text of texts) {
    for (const c of citations(text)) {
      if (!ids.includes(c.id) && ids.length < maxCauseFacts) {
        ids.push(c.id)
      }
    }
  }
  return ids
}

function causeInputs(
  input: unknown
): { causes: CauseInput[]; ruledOut: string[]; given: boolean } | undefined {
  const in_ = parseCauses(input)
  if (!in_) {
    return undefined
  }
  if (in_.causes.length > 0 || !in_.rootCause) {
    return { causes: in_.causes, ruledOut: in_.ruledOut, given: true }
  }
  const legacy: CauseInput = {
    component: in_.rootCause.component,
    onset: in_.onset,
    facts: citedFacts(in_.markdown, in_.impact, in_.rootCause.summary),
    traceIDs: [],
  }
  return { causes: [legacy], ruledOut: in_.ruledOut, given: false }
}

export function signalOf(f: Fact): string {
  const tool = f.of.split(" ")[0] ?? ""
  if (f.signal) {
    return f.signal
  }
  if (
    f.kind === "trace_id" ||
    tool.startsWith("trace_") ||
    tool === "get_trace"
  ) {
    return signalTraces
  }
  if (tool.startsWith("log")) {
    return signalLogs
  }
  return signalMetrics
}

function agreesWith(f: Fact, onset: number): boolean {
  const at = parseTime(f.at ?? "")
  if (at !== undefined) {
    return Math.abs(at - onset) <= onsetAgreement
  }
  const from = parseTime(f.from ?? "")
  const to = parseTime(f.to ?? "")
  return from !== undefined && to !== undefined && onset >= from && onset <= to
}

function signalsOf(
  r: GroundingRecord,
  c: CauseInput,
  ledger: Ledger,
  known: readonly string[]
): Signals {
  const s: Signals = {
    kinds: [],
    component: false,
    onset_read: false,
    onset_fact: false,
    trigger_read: false,
    ruled_out: false,
  }
  const addKind = (kind: string) => {
    if (!s.kinds.includes(kind)) {
      s.kinds.push(kind)
    }
  }
  const cited: Fact[] = []
  for (const id of c.facts) {
    const f = ledger.fact(trimSpace(id))
    if (f) {
      cited.push(f)
      addKind(signalOf(f))
    }
  }
  for (const id of c.traceIDs) {
    if (known.includes(toLower(trimSpace(id)))) {
      addKind(signalTraces)
    }
  }
  s.component = trimSpace(c.component) !== "" && sawName(r, c.component)
  if (c.trigger !== undefined && trimSpace(c.trigger) !== "") {
    s.trigger_read = sawName(r, c.trigger)
  }
  const at = parseTime(trimSpace(c.onset))
  if (at !== undefined) {
    s.onset_read = queried(r, at)
    s.onset_fact = cited.some((f) => agreesWith(f, at))
  }
  return s
}

export function causeSignals(
  r: GroundingRecord | undefined,
  input: unknown
): Signals[] | undefined {
  if (!r) {
    return undefined
  }
  const in_ = causeInputs(input)
  if (!in_) {
    return undefined
  }
  const ledger = ledgerOf(r) as Ledger
  const known = knownIDs(r, ledger)
  const ruledOut = in_.ruledOut.some((line) =>
    citations(line).some((c) => ledger.fact(c.id) !== undefined)
  )
  return in_.causes.map((c) => ({
    ...signalsOf(r, c, ledger, known),
    ruled_out: ruledOut,
  }))
}

function namedProblems(r: GroundingRecord, c: CauseInput): Problem[] {
  const out: Problem[] = []
  const name = trimSpace(c.component)
  if (name !== "" && !sawName(r, name)) {
    out.push({ kind: kinds.component, text: name, fix: itemAction })
  }
  if (
    c.trigger !== undefined &&
    trimSpace(c.trigger) !== "" &&
    !sawName(r, c.trigger)
  ) {
    out.push({
      kind: kinds.trigger,
      text: trimSpace(c.trigger),
      fix: "read where it is set, or name it exactly as a result spelled it, or leave the trigger out",
    })
  }
  const onset = trimSpace(c.onset)
  if (onset !== "") {
    const when = parseTime(onset)
    if (when !== undefined && !queried(r, when)) {
      out.push({ kind: kinds.onset, text: onset, fix: itemAction })
    }
  }
  return out
}

function evidenceProblems(
  at: string,
  c: CauseInput,
  ledger: Ledger,
  known: readonly string[]
): Problem[] {
  const out: Problem[] = []
  for (const id of c.facts) {
    if (!ledger.fact(trimSpace(id))) {
      out.push({
        kind: kinds.fact,
        text: at + ".evidence.facts: " + trimSpace(id),
        fix: "not a fact id a result of this run gave: cite one that was, or remove it",
      })
    }
  }
  for (const id of c.traceIDs) {
    if (!known.includes(toLower(trimSpace(id)))) {
      out.push({
        kind: kinds.traceID,
        text: trimSpace(id),
        fix: "not an id this run read: read the trace, cite an id a result gave, or remove it",
      })
    }
  }
  return out
}

export function causeProblems(r: GroundingRecord, input: unknown): Problem[] {
  const in_ = causeInputs(input)
  if (!in_ || !in_.given || in_.causes.length === 0) {
    return []
  }
  const ledger = ledgerOf(r) as Ledger
  const known = knownIDs(r, ledger)
  return in_.causes.flatMap((c, i) => [
    ...namedProblems(r, c),
    ...evidenceProblems(`causes[${i}]`, c, ledger, known),
  ])
}

export const bandHigh = "high"
export const bandMedium = "medium"
export const bandLow = "low"
export const highFrom = 0.7
export const statedFrom = 0.4

export function score(s: Signals): number {
  let count = 0
  for (const k of [signalMetrics, signalLogs, signalTraces]) {
    if (s.kinds.includes(k)) {
      count++
    }
  }
  let total = 0
  if (count > 0) {
    total += 0.25 + (count - 1) * 0.15
  }
  if (s.component) {
    total += 0.1
  }
  if (s.onset_fact) {
    total += 0.15
  } else if (s.onset_read) {
    total += 0.05
  }
  if (s.trigger_read) {
    total += 0.2
  }
  if (s.ruled_out) {
    total += 0.05
  }
  if (count === 0) {
    total = Math.min(total, 0.3)
  }
  return Math.round(Math.min(total, 1) * 100) / 100
}

export function bandOf(score: number): string {
  if (score >= highFrom) {
    return bandHigh
  }
  if (score >= statedFrom) {
    return bandMedium
  }
  return bandLow
}

export function queriesOf(ledger: Ledger, facts: readonly string[]): string[] {
  const ids: string[] = []
  for (const raw of facts) {
    const id = trimSpace(raw)
    if (!ids.includes(id)) {
      ids.push(id)
    }
  }
  const out: string[] = []
  for (const id of ids) {
    const q = clipBytes(trimSpace(ledger.queryOf(id)), maxQueryBytes)
    if (q !== "" && !out.includes(q) && out.length < maxCauseQueries) {
      out.push(q)
    }
  }
  return out
}

export type ScoredCause = {
  signals: Signals
  score: number
  band: string
  queries: string[]
}

export function scoredCauses(
  r: GroundingRecord | undefined,
  input: unknown
): ScoredCause[] | undefined {
  const signals = causeSignals(r, input)
  const in_ = causeInputs(input)
  if (!signals || !in_ || !in_.given) {
    return undefined
  }
  const ledger = ledgerOf(r) as Ledger
  return in_.causes.map((c, i) => {
    const s = signals[i] as Signals
    const value = score(s)
    return {
      signals: s,
      score: value,
      band: bandOf(value),
      queries: queriesOf(ledger, c.facts),
    }
  })
}
