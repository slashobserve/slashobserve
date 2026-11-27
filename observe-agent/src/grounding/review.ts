import { compareBytes, goQuote, toLower, trimSpace } from "../brief/gofmt"
import { causeProblems } from "./causes"
import { blank, type Figure, scalesOf, scan } from "./figures"
import { type FindingInput, parseReviewed } from "./input"
import { uncitedFigures } from "./needfact"
import { clip, itemAction, kinds, type Problem, Problems } from "./problems"
import {
  type Fact,
  type GroundingRecord,
  minNameRunes,
  queried,
} from "./record"
import {
  citations,
  Ledger,
  relative,
  renderTolerance,
  withoutCitations,
} from "./render"
import { parseTime } from "./time"
import { traceIDProblems } from "./traceids"

const tolerance = 0.02
const maxTolerance = 0.1

export type Answer = { quantity: string; fact: string }

export function ledgerOf(r: GroundingRecord | undefined): Ledger | undefined {
  return r ? new Ledger([...r.facts]) : undefined
}

function spreadOf(f: Figure): number {
  return Math.min(
    Math.max(tolerance * f.value, f.step / 2),
    maxTolerance * f.value
  )
}

function matching(
  l: Ledger | undefined,
  f: Figure
): { exact?: Fact; near?: Fact } {
  if (!l) {
    return {}
  }
  const spread = spreadOf(f)
  let near: Fact | undefined
  for (const fact of l.facts) {
    if (fact.kind === "trace_id") {
      continue
    }
    const v = Math.abs(fact.value ?? 0)
    for (const scale of scalesOf(f.unit)) {
      const off = Math.abs(f.value * scale - v)
      if (off <= (f.step / 2) * scale && off <= renderTolerance * v) {
        return { exact: fact }
      }
      if (near === undefined && off <= spread * scale) {
        near = fact
      }
    }
  }
  return near ? { near } : {}
}

type Interval = { lo: number; hi: number }

const unitless = new Set(["", "%", "x"])

function intervalsOf(f: Figure): Interval[] {
  const spread = spreadOf(f)
  return scalesOf(f.unit).map((scale) => ({
    lo: (f.value - spread) * scale,
    hi: (f.value + spread) * scale,
  }))
}

function lowerBound(sorted: readonly number[], x: number): number {
  let lo = 0
  let hi = sorted.length
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if ((sorted[mid] as number) < x) {
      lo = mid + 1
    } else {
      hi = mid
    }
  }
  return lo
}

function anyIn(sorted: readonly number[], lo: number, hi: number): boolean {
  const i = lowerBound(sorted, lo)
  return i < sorted.length && (sorted[i] as number) <= hi
}

function otherIn(
  sorted: readonly number[],
  self: number,
  lo: number,
  hi: number
): boolean {
  for (
    let i = lowerBound(sorted, lo);
    i < sorted.length && (sorted[i] as number) <= hi;
    i++
  ) {
    if (sorted[i] !== self) {
      return true
    }
  }
  return false
}

function derived(values: readonly number[], at: Interval): boolean {
  if (at.hi <= 0) {
    return false
  }
  const lo = Math.max(at.lo, 0)
  return values.some(
    (a) =>
      otherIn(values, a, a - at.hi, a - lo) ||
      (a <= at.hi && otherIn(values, a, lo - a, at.hi - a))
  )
}

function ratioIn(values: readonly number[], at: Interval): boolean {
  if (at.hi <= 0) {
    return false
  }
  const upper = at.lo > 0 ? 1 / at.lo : Infinity
  return values.some((a) => a > 0 && otherIn(values, a, a / at.hi, a * upper))
}

type SortedReads = { all: number[]; reads: number[][] }

const numeric = (a: number, b: number) => a - b

function sortedReads(r: GroundingRecord): SortedReads {
  return {
    reads: r.reads.map((read) => [...read].sort(numeric)),
    all: r.reads.flat().sort(numeric),
  }
}

function found(s: SortedReads, f: Figure): boolean {
  const intervals = intervalsOf(f)
  if (intervals.some((at) => anyIn(s.all, at.lo, at.hi))) {
    return true
  }
  if (f.unit === "%") {
    const fraction = intervals[1] as Interval
    const up = { lo: 1 + fraction.lo, hi: 1 + fraction.hi }
    const down = { lo: 1 - fraction.hi, hi: 1 - fraction.lo }
    return s.reads.some(
      (read) =>
        derived(read, fraction) ||
        ratioIn(read, fraction) ||
        ratioIn(read, up) ||
        ratioIn(read, down)
    )
  }
  const ratios = unitless.has(f.unit)
  return s.reads.some((read) =>
    intervals.some((at) => derived(read, at) || (ratios && ratioIn(read, at)))
  )
}

const genericTokens = new Set([
  "the",
  "service",
  "svc",
  "server",
  "api",
  "app",
  "pod",
  "pods",
  "deployment",
  "container",
])

function tokensOf(name: string): string[] {
  return toLower(name)
    .split(/[^\p{L}\p{Nd}]+/u)
    .filter((t) => t !== "")
}

function containsRun(
  seen: readonly string[],
  want: readonly string[]
): boolean {
  for (let i = 0; i + want.length <= seen.length; i++) {
    if (want.every((w, j) => seen[i + j] === w)) {
      return true
    }
  }
  return false
}

export function sawName(r: GroundingRecord, component: string): boolean {
  let want = tokensOf(component)
  const specific = want.filter((t) => !genericTokens.has(t))
  if (specific.length > 0) {
    want = specific
  }
  const whole = want.join("")
  if (whole === "") {
    return true
  }
  const byTokens = [...whole].length >= minNameRunes
  for (const name of [...r.discovered, ...r.names, ...r.sourced]) {
    const seen = tokensOf(name)
    if (seen.join("") === whole || (byTokens && containsRun(seen, want))) {
      return true
    }
  }
  return false
}

export function knownIDs(r: GroundingRecord, l: Ledger | undefined): string[] {
  const ids = [...r.ids]
  for (const f of l?.facts ?? []) {
    if (f.kind === "trace_id") {
      ids.push(f.text)
    }
  }
  return ids
}

function normalQuantity(q: string): string {
  return toLower(q)
    .split(/[^\p{L}\p{Nd}]+/u)
    .filter((t) => t !== "")
    .join(" ")
}

function agree(a: Fact, b: Fact): boolean {
  if (a.id === b.id) {
    return true
  }
  if (a.kind === "trace_id" || b.kind === "trace_id") {
    return a.text === b.text
  }
  const av = a.value ?? 0
  const bv = b.value ?? 0
  return (
    relative(av, bv) <= renderTolerance && relative(bv, av) <= renderTolerance
  )
}

export function answerProblems(
  l: Ledger | undefined,
  answers: readonly Answer[]
): Problem[] {
  const problems: Problem[] = []
  const seen: { quantity: string; fact: Fact }[] = []
  for (const a of answers) {
    const quantity = normalQuantity(a.quantity)
    const f = l?.fact(trimSpace(a.fact))
    if (!f) {
      problems.push({
        kind: kinds.fact,
        text: `answers: ${clip(a.quantity)} = ${a.fact}`,
        fix: `${a.fact} is not a fact this run read: name the fact id a result gave for this quantity`,
      })
      continue
    }
    const prior = seen.find((e) => e.quantity === quantity)
    if (prior && !agree(prior.fact, f)) {
      problems.push({
        kind: kinds.fact,
        text: `answers: ${clip(a.quantity)} = ${prior.fact.id} or ${f.id}`,
        fix: "one quantity takes one fact: make the read that settles which value is right and list only that fact",
      })
      continue
    }
    if (!prior) {
      seen.push({ quantity, fact: f })
    }
  }
  return problems
}

function question(f: Fact): string {
  return f.asked || f.query || ""
}

function quantityKey(f: Fact): string {
  const traceLabel = f.labels?.["trace_id"] ?? ""
  if (
    f.kind === "duration" &&
    traceLabel !== "" &&
    f.of.endsWith("duration_ms")
  ) {
    return "trace " + traceLabel.toLowerCase() + " duration"
  }
  if (f.kind === "trace_id") {
    return ""
  }
  const parts = [f.of, f.from ?? "", f.to ?? "", f.at ?? "", question(f)]
  for (const k of Object.keys(f.labels ?? {}).sort(compareBytes)) {
    parts.push(k + "=" + (f.labels?.[k] ?? ""))
  }
  return parts.join("\u0000")
}

export function conflicts(
  l: Ledger | undefined,
  ...texts: string[]
): Problem[] {
  const cited: Fact[] = []
  for (const text of texts) {
    for (const c of citations(text)) {
      const f = l?.fact(c.id)
      if (f && !cited.some((g) => g.id === f.id)) {
        cited.push(f)
      }
    }
  }
  const problems: Problem[] = []
  cited.forEach((a, i) => {
    const key = quantityKey(a)
    if (key === "") {
      return
    }
    for (const b of cited.slice(0, i)) {
      if (quantityKey(b) === key && !agree(a, b)) {
        problems.push({
          kind: kinds.fact,
          text: `{{${b.id}}} and {{${a.id}}}`,
          fix: `both are ${a.of} for the same labels and window, ${b.text} and ${a.text}: make one more read that settles it and cite only that fact`,
        })
        break
      }
    }
  })
  return problems
}

function stated(f: Fact, texts: readonly string[]): boolean {
  const one = new Ledger([f])
  for (const text of texts) {
    if (citations(text).some((c) => c.id === f.id)) {
      return true
    }
    if (f.kind === "trace_id") {
      if (toLower(text).includes(f.text)) {
        return true
      }
      continue
    }
    for (const figure of scan(blank(withoutCitations(text)))) {
      if (matching(one, figure).exact) {
        return true
      }
    }
  }
  return false
}

export function unstated(
  l: Ledger | undefined,
  answers: readonly Answer[],
  ...texts: string[]
): Problem[] {
  const out: Problem[] = []
  for (const a of answers) {
    const f = l?.fact(trimSpace(a.fact))
    if (!f || stated(f, texts)) {
      continue
    }
    out.push({
      kind: kinds.fact,
      text: `answers: ${a.quantity} = ${f.id}`,
      fix: `the markdown never gives this answer: cite {{${f.id}}} where it answers ${goQuote(a.quantity)}`,
    })
  }
  return out
}

function figureProblems(
  l: Ledger | undefined,
  seen: SortedReads,
  text: string,
  q: string
): Problem[] {
  const out: Problem[] = []
  for (const f of uncitedFigures(text, q)) {
    const { exact, near } = matching(l, f)
    if (exact) {
      continue
    }
    if (near) {
      out.push({
        kind: kinds.figure,
        text: f.text,
        fix: `more than 1% from ${near.id}, ${near.text}: cite {{${near.id}}}, with significant figures that keep it within 1%`,
      })
    } else if (!found(seen, f)) {
      out.push({ kind: kinds.figure, text: f.text, fix: itemAction })
    }
  }
  return out
}

function nameAndTimeProblems(r: GroundingRecord, in_: FindingInput): Problem[] {
  const out: Problem[] = []
  const component = trimSpace(in_.root_cause?.component ?? "")
  if (
    in_.root_cause &&
    component !== "" &&
    !sawName(r, in_.root_cause.component ?? "")
  ) {
    out.push({ kind: kinds.component, text: component, fix: itemAction })
  }
  const onset = trimSpace(in_.onset ?? "")
  if (onset !== "") {
    const at = parseTime(onset)
    if (at !== undefined && !queried(r, at)) {
      out.push({ kind: kinds.onset, text: onset, fix: itemAction })
    }
  }
  return out
}

function textsOf(in_: FindingInput): string[] {
  const texts = [in_.title ?? "", in_.markdown ?? "", in_.impact ?? ""]
  if (in_.root_cause) {
    texts.push(in_.root_cause.summary ?? "")
  }
  for (const c of in_.causes ?? []) {
    texts.push(c.mechanism ?? "")
  }
  return [...texts, ...(in_.ruled_out ?? []), ...(in_.next_steps ?? [])]
}

export function review(
  r: GroundingRecord | undefined,
  input: unknown,
  q: string
): Problem[] {
  if (!r) {
    return []
  }
  const in_ = parseReviewed(input)
  if (!in_) {
    return []
  }
  const ledger = ledgerOf(r)
  const out = new Problems()
  for (const text of textsOf(in_)) {
    out.add(...(ledger as Ledger).render(text).problems)
  }
  const answers = in_.answers ?? []
  const markdown = in_.markdown ?? ""
  const impact = in_.impact ?? ""
  out.add(...answerProblems(ledger, answers))
  out.add(...unstated(ledger, answers, markdown, impact))
  out.add(...conflicts(ledger, markdown, impact))
  const seen = sortedReads(r)
  for (const text of [markdown, impact]) {
    out.add(...figureProblems(ledger, seen, text, q))
  }
  const known = knownIDs(r, ledger)
  for (const text of [markdown, impact]) {
    out.add(...traceIDProblems(text, known))
  }
  out.add(...nameAndTimeProblems(r, in_))
  out.add(...causeProblems(r, input))
  return out.list
}

const missingAnswersFix =
  "the markdown states a figure but answers is empty: list each quantity the question asks for in answers with the one fact that answers it"

export function missingAnswers(input: unknown, q: string): Problem | undefined {
  const in_ = parseReviewed(input)
  if (!in_ || (in_.answers ?? []).length > 0) {
    return undefined
  }
  for (const text of [in_.markdown ?? "", in_.impact ?? ""]) {
    if (citations(text).length > 0 || uncitedFigures(text, q).length > 0) {
      const line = trimSpace(in_.markdown ?? "").split("\n")[0] ?? ""
      return {
        kind: kinds.answers,
        text: trimSpace(clip(line)),
        fix: missingAnswersFix,
      }
    }
  }
  return undefined
}
