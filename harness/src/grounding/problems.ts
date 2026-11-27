import { goQuote } from "../brief/gofmt"

export const kinds = {
  figure: "figure",
  component: "component",
  onset: "onset",
  fact: "fact",
  traceID: "trace_id",
  trigger: "trigger",
  candidate: "candidate",
  answers: "answers",
} as const

export const maxUnverified = 20

const maxItemRunes = 120

export const itemAction = "read it, correct it, or remove it"

const reviewLead =
  "Some figures, facts, ids or names in this finding do not match what this run read. Fix each item below as it says, reading again first if you need to, then call save_finding again."

export type Problem = { kind: string; text: string; fix: string }

export type Unverified = { kind: string; text: string }

export function clip(text: string): string {
  const runes = [...text]
  return runes.length <= maxItemRunes
    ? text
    : runes.slice(0, maxItemRunes).join("")
}

export class Problems {
  readonly list: Problem[] = []

  add(...more: Problem[]): void {
    for (const p of more) {
      const item = { ...p, text: clip(p.text) }
      if (
        this.list.length < maxUnverified &&
        !this.list.some((q) => q.kind === item.kind && q.text === item.text)
      ) {
        this.list.push(item)
      }
    }
  }
}

export function unverifiedOf(ps: readonly Problem[]): Unverified[] {
  return ps.map((p) => ({ kind: p.kind, text: p.text }))
}

function describe(kind: string): string {
  switch (kind) {
    case kinds.component:
      return "component, not a name this run saw,"
    case kinds.trigger:
      return "trigger key, not a name this run saw,"
    case kinds.onset:
      return "onset, outside every time range this run queried,"
    case kinds.fact:
      return "cited fact"
    case kinds.traceID:
      return "trace id"
    case kinds.answers:
      return "answer"
  }
  return "figure"
}

export function sendBackProblems(ps: readonly Problem[]): string {
  return [
    reviewLead,
    ...ps.map((p) => `- ${describe(p.kind)} ${goQuote(p.text)}: ${p.fix}`),
  ].join("\n")
}
