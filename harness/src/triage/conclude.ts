import { z } from "zod"
import { byteLength, toLower, trimSpace } from "../brief/gofmt"
import type { Hypothesis } from "../brief/hypotheses"
import type { Task } from "../route/route"
import type { Lead } from "./types"

const unclosedLead =
  "the finding cannot conclude while a triage candidate is unaccounted for: "
const unclosedTail =
  ". Close each one in note_hypotheses as cause, knock_on with knock_on_of, background, transient or unrelated, with the evidence you read, then save the finding again"

const minReadsForTransient = 2
const maxCandidateText = 120

const hypothesisOpen = "open"
const closingCause = "cause"
const closingKnockOn = "knock_on"
const closingTransient = "transient"
const excludedClosings = ["background", "transient", "unrelated"]

export function worksCandidates(task: Task): boolean {
  return task === "incident" || task === ""
}

export function leadsOf(
  loop: boolean,
  task: Task,
  leads: readonly Lead[] | undefined
): Lead[] {
  return loop && worksCandidates(task) ? [...(leads ?? [])] : []
}

export function seedLedger(
  leads: readonly Lead[],
  ledger: readonly Hypothesis[]
): Hypothesis[] | undefined {
  if (leads.length === 0 || ledger.length > 0) {
    return undefined
  }
  return leads.map((l) => ({
    id: l.id,
    text: l.text,
    status: hypothesisOpen,
    next: l.next,
  }))
}

function sameHypothesis(a: Hypothesis, b: Hypothesis): boolean {
  const ae = a.evidence ?? []
  const be = b.evidence ?? []
  return (
    a.id === b.id &&
    a.text === b.text &&
    a.status === b.status &&
    (a.closing ?? "") === (b.closing ?? "") &&
    (a.knock_on_of ?? "") === (b.knock_on_of ?? "") &&
    (a.next ?? "") === (b.next ?? "") &&
    ae.length === be.length &&
    ae.every((line, i) => line === be[i])
  )
}

export function noteTransient(
  leads: readonly Lead[],
  ledger: readonly Hypothesis[],
  next: readonly Hypothesis[],
  transientAt: Readonly<Record<string, number>>,
  dataReads: number
): Record<string, number> {
  const out = { ...transientAt }
  for (const l of leads) {
    const h = next.find((x) => x.id === l.id)
    if (!h || h.closing !== closingTransient) {
      delete out[l.id]
      continue
    }
    const prior = ledger.find((x) => x.id === l.id)
    if (Object.hasOwn(out, l.id) && prior && sameHypothesis(prior, h)) {
      continue
    }
    out[l.id] = dataReads
  }
  return out
}

export type Concluded = { noIncident: boolean; names: string[] }

const concludedSchema = z.object({
  no_incident: z.boolean().nullish(),
  root_cause: z.object({ component: z.string().nullish() }).nullish(),
  causes: z
    .array(
      z
        .object({
          component: z.object({ name: z.string().nullish() }).nullish(),
        })
        .nullable()
    )
    .nullish(),
})

export function concludedOf(input: unknown): Concluded {
  const parsed = concludedSchema.safeParse(input ?? {})
  if (!parsed.success) {
    return { noIncident: false, names: [] }
  }
  const v = parsed.data
  const causes = v.causes ?? []
  const names = causes.map((c) => toLower(trimSpace(c?.component?.name ?? "")))
  if (v.root_cause && causes.length === 0) {
    names.push(toLower(trimSpace(v.root_cause.component ?? "")))
  }
  return { noIncident: v.no_incident ?? false, names }
}

function blames(c: Concluded, service: string): boolean {
  return c.names.includes(toLower(service))
}

function openProblem(
  name: string,
  l: Lead,
  h: Hypothesis | undefined,
  transientAt: Readonly<Record<string, number>>
): string {
  if (!h) {
    return name + " is missing from the ledger; send it again with the others"
  }
  if (h.status === hypothesisOpen) {
    return name + " is still open; next: " + (h.next || l.next)
  }
  if (!h.closing) {
    return name + " is " + h.status + " with no closing"
  }
  if (
    h.closing === closingTransient &&
    (transientAt[l.id] ?? 0) < minReadsForTransient
  ) {
    return (
      name +
      " is closed as transient, but no read after the first one shows it gone; read it again and cite that later read"
    )
  }
  return ""
}

function closedProblem(
  name: string,
  l: Lead,
  h: Hypothesis,
  sameService: readonly Hypothesis[],
  f: Concluded
): string {
  const counted = sameService.some(
    (o) => o.closing === closingCause || o.closing === closingKnockOn
  )
  const closing = h.closing ?? ""
  if (
    f.noIncident &&
    (closing === closingCause || closing === closingKnockOn)
  ) {
    return "no_incident is set, but " + name + " is closed as " + closing
  }
  if (f.noIncident) {
    return ""
  }
  if (closing === closingCause && !blames(f, l.service)) {
    return name + " is closed as cause, but causes does not name " + l.service
  }
  if (excludedClosings.includes(closing) && !counted && blames(f, l.service)) {
    return (
      "causes names " + l.service + ", which " + l.id + " closed as " + closing
    )
  }
  return ""
}

export function candidateProblems(
  leads: readonly Lead[],
  ledger: readonly Hypothesis[],
  transientAt: Readonly<Record<string, number>>,
  f: Concluded
): { problems: string[]; open: Lead[] } {
  const byService = new Map<string, Hypothesis[]>()
  for (const l of leads) {
    const h = ledger.find((x) => x.id === l.id)
    if (h) {
      const key = toLower(l.service)
      byService.set(key, [...(byService.get(key) ?? []), h])
    }
  }
  const problems: string[] = []
  const open: Lead[] = []
  for (const l of leads) {
    const name = l.id + " (" + l.service + ")"
    const h = ledger.find((x) => x.id === l.id)
    const problem = openProblem(name, l, h, transientAt)
    if (problem !== "") {
      problems.push(problem)
      open.push(l)
      continue
    }
    const closed = closedProblem(
      name,
      l,
      h as Hypothesis,
      byService.get(toLower(l.service)) ?? [],
      f
    )
    if (closed !== "") {
      problems.push(closed)
    }
  }
  return { problems, open }
}

function clipWithDots(s: string, n: number): string {
  if (byteLength(s) <= n) {
    return s
  }
  const raw = Buffer.from(s, "utf8")
  let cut = n
  while (cut > 0 && ((raw[cut] as number) & 0xc0) === 0x80) {
    cut--
  }
  return raw.subarray(0, cut).toString("utf8") + "..."
}

export type CandidateCheck = {
  unclosed: string
  unverified: { kind: string; text: string }[]
}

export function checkCandidates(
  leads: readonly Lead[],
  ledger: readonly Hypothesis[],
  transientAt: Readonly<Record<string, number>>,
  input: unknown
): CandidateCheck {
  if (leads.length === 0) {
    return { unclosed: "", unverified: [] }
  }
  const { problems, open } = candidateProblems(
    leads,
    ledger,
    transientAt,
    concludedOf(input)
  )
  return {
    unclosed:
      problems.length > 0
        ? unclosedLead + problems.join("; ") + unclosedTail
        : "",
    unverified: open.map((l) => ({
      kind: "candidate",
      text: clipWithDots(l.id + " " + l.service, maxCandidateText),
    })),
  }
}
