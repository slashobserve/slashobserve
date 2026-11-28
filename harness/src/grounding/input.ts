import { z } from "zod"

const text = z.string().nullish()
const texts = z.array(z.string().nullable()).nullish()
const flag = z.boolean().nullish()

function strings(
  list: readonly (string | null)[] | null | undefined
): string[] {
  return (list ?? []).map((s) => s ?? "")
}

const reviewedSchema = z.object({
  title: text,
  markdown: text,
  impact: text,
  onset: text,
  root_cause: z.object({ component: text, summary: text }).nullish(),
  answers: z
    .array(z.object({ quantity: text, fact: text }).nullable())
    .nullish(),
  ruled_out: texts,
  next_steps: texts,
  causes: z.array(z.object({ mechanism: text }).nullable()).nullish(),
})

export type FindingInput = {
  title?: string
  markdown?: string
  impact?: string
  onset?: string
  root_cause?: { component?: string; summary?: string }
  answers?: { quantity: string; fact: string }[]
  ruled_out?: string[]
  next_steps?: string[]
  causes?: { mechanism?: string }[]
}

export function parseReviewed(input: unknown): FindingInput | undefined {
  const parsed = reviewedSchema.safeParse(input ?? {})
  if (!parsed.success) {
    return undefined
  }
  const v = parsed.data
  const out: FindingInput = {
    title: v.title ?? "",
    markdown: v.markdown ?? "",
    impact: v.impact ?? "",
    onset: v.onset ?? "",
    answers: (v.answers ?? []).map((a) => ({
      quantity: a?.quantity ?? "",
      fact: a?.fact ?? "",
    })),
    ruled_out: strings(v.ruled_out),
    next_steps: strings(v.next_steps),
    causes: (v.causes ?? []).map((c) => ({ mechanism: c?.mechanism ?? "" })),
  }
  if (v.root_cause) {
    out.root_cause = {
      component: v.root_cause.component ?? "",
      summary: v.root_cause.summary ?? "",
    }
  }
  return out
}

const component = z
  .object({ kind: text, name: text, namespace: text })
  .nullish()

const causeSchema = z
  .object({
    component,
    kind: text,
    mechanism: text,
    trigger: z.object({ kind: text, key: text, value: text }).nullish(),
    onset: text,
    affected: z.array(component).nullish(),
    knock_on_of: z.number().int().nullish(),
    evidence: z.object({ facts: texts, trace_ids: texts }).nullish(),
    estimate: text,
    signals: z
      .object({
        kinds: texts,
        component: flag,
        onset_read: flag,
        onset_fact: flag,
        trigger_read: flag,
        ruled_out: flag,
      })
      .nullish(),
    score: z.number().nullish(),
    band: text,
    queries: texts,
  })
  .nullable()

const causesSchema = z.object({
  markdown: text,
  root_cause: z
    .object({ component: text, kind: text, summary: text })
    .nullish(),
  onset: text,
  impact: text,
  confidence: text,
  ruled_out: texts,
  next_steps: texts,
  no_incident: flag,
  causes: z.array(causeSchema).nullish(),
})

export type CauseInput = {
  component: string
  trigger?: string
  onset: string
  facts: string[]
  traceIDs: string[]
}

export type CausesInput = {
  markdown: string
  impact: string
  onset: string
  ruledOut: string[]
  rootCause?: { component: string; summary: string }
  causes: CauseInput[]
}

export function parseCauses(input: unknown): CausesInput | undefined {
  const parsed = causesSchema.safeParse(input ?? {})
  if (!parsed.success) {
    return undefined
  }
  const v = parsed.data
  const out: CausesInput = {
    markdown: v.markdown ?? "",
    impact: v.impact ?? "",
    onset: v.onset ?? "",
    ruledOut: strings(v.ruled_out),
    causes: (v.causes ?? []).map((c) => {
      const cause: CauseInput = {
        component: c?.component?.name ?? "",
        onset: c?.onset ?? "",
        facts: strings(c?.evidence?.facts),
        traceIDs: strings(c?.evidence?.trace_ids),
      }
      if (c?.trigger) {
        cause.trigger = c.trigger.key ?? ""
      }
      return cause
    }),
  }
  if (v.root_cause) {
    out.rootCause = {
      component: v.root_cause.component ?? "",
      summary: v.root_cause.summary ?? "",
    }
  }
  return out
}
