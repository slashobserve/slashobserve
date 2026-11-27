import type { RunBrief } from "../protocol"
import { buildSystem, investigateFor } from "../prompts/run"
import type { Task } from "../route/route"
import { anchorNote, type Pointer } from "./anchor"
import {
  buildContext,
  type Attention,
  type Dashboard,
  investigateContext,
  type Revision,
  type Unverified,
} from "./context"
import { digestText, type Discovery } from "./digest"
import {
  formatClock,
  formatDate,
  formatMinute,
  formatRFC3339,
  unixSeconds,
} from "./gofmt"
import { hypothesesText, type Hypothesis } from "./hypotheses"
import type { Reason, Seed } from "./seed"

export type BriefState = {
  task: Task
  loop: boolean
  facts: boolean
  ledger: readonly Hypothesis[]
  triage: string
  digest: readonly Discovery[]
}

export type Message = {
  system: string
  context: string
  user: string
}

const threadLead =
  "The request was asked in this thread (earlier messages, oldest first):"

const factsNote =
  "When you answer with save_finding, write each figure from a read that gave fact ids in `facts` as its fact, `{{f3}}`, which the server fills in (`{{f3:ms}}`, `{{f3:%}}` or `{{f3:3}}` change the unit or the figures within 1%), copy trace ids whole, and list each quantity asked in `answers` with its one fact."

const withheldNote =
  "Audit data, other datasources only Admins can query and what is stored in folders only Admins can read are available in a private chat, or in a folder only Admins can read. " +
  "What this run writes is read by people who are not Admins, so they are not available to it."

const withheldAdvice =
  "When the request needs them, say so in your answer and do not guess their contents."

const windowAdvice =
  "take your window from that time and state the window you queried."

type Zone = { label: string; offset_seconds: number }

export function clockNote(
  name: string,
  zone: Zone | null | undefined,
  at: number
): string {
  const utc = formatMinute(at)
  if (!zone) {
    return `It is now ${utc} UTC; ${windowAdvice}`
  }
  const local = at + zone.offset_seconds
  const note = `Time zone: the person reads times in ${name}, labelled ${zone.label}. Tool results give UTC; write every time in the answer in ${name} with its label, such as 14:05 ${zone.label}, not in UTC. It is now ${formatClock(local)} ${zone.label} on ${formatDate(local)}`
  if (zone.label === "UTC") {
    return note + "; " + windowAdvice
  }
  return `${note}, which is ${utc} UTC; ${windowAdvice}`
}

function asOfNote(at: number): string {
  return `Now is ${formatRFC3339(at)} for this run: every relative time, such as now-1h, resolves to it, and the data may end there.`
}

export function userMessage(context: string, prompt: string): string {
  return context === ""
    ? prompt
    : context + "\n\n<request>\n" + prompt + "\n</request>"
}

function revisionOf(current: NonNullable<RunBrief["current"]>): Revision {
  return {
    dashboardID: current.dashboard_id,
    number: current.number,
    spec: current.spec as Dashboard,
    unverified: current.unverified as Unverified,
    conversion: (current.conversion ?? []) as Attention[],
  }
}

function buildView(
  current: Revision | null,
  build: NonNullable<RunBrief["build"]>
): Revision {
  return {
    dashboardID: current?.dashboardID ?? "",
    number: current?.number ?? 0,
    spec: build.spec as Dashboard,
    unverified: build.unverified as Unverified,
    conversion: current?.conversion ?? [],
  }
}

export function render(
  brief: RunBrief,
  reason: Reason,
  state: BriefState,
  now: Date
): Message {
  const r = brief.request
  const current = brief.current ? revisionOf(brief.current) : null
  const seed: Seed = {
    progress: brief.build ? brief.build.progress : null,
    reason,
    note: r.continue_note ?? "",
  }
  let system: string
  let context: string
  if (r.mode === "investigate") {
    system = investigateFor(
      { task: state.task, facts: state.facts, ledger: state.loop },
      brief.guide.text
    )
    context = investigateContext({
      statuses: brief.datasources,
      from: r.from ?? null,
      to: r.to ?? null,
      panelID: r.panel_id ?? "",
      prompt: r.prompt,
      current,
      seed,
      triage: state.triage,
    })
  } else {
    system = buildSystem(brief.guide.text)
    context = buildContext({
      statuses: brief.datasources,
      mode: r.mode,
      source: r.source,
      panelID: r.panel_id ?? "",
      prompt: r.prompt,
      current: brief.build ? buildView(current, brief.build) : current,
      hasBuild: Boolean(brief.build),
      seed,
    })
  }
  const at = r.as_of ? unixSeconds(r.as_of) : Math.floor(now.getTime() / 1000)
  if (r.mode !== "suggest") {
    context += "\n\n" + clockNote(r.time_zone ?? "", r.zone, at)
    if (state.facts && r.mode !== "investigate") {
      context += "\n\n" + factsNote
    }
  }
  const note = anchorNote(r.pointer as Pointer)
  if (note !== "") {
    context += "\n\n" + note
  }
  if (r.thread_context) {
    context += "\n\n" + threadLead + "\n" + r.thread_context
  }
  if (brief.withheld) {
    context += "\n\n" + withheldNote + " " + withheldAdvice
  }
  if (r.as_of) {
    context += "\n\n" + asOfNote(unixSeconds(r.as_of))
  }
  const digest = digestText(state.digest)
  if (digest !== "" && r.mode !== "suggest") {
    context += "\n\n" + digest
  }
  const hypotheses = state.loop ? hypothesesText(state.ledger) : ""
  if (hypotheses !== "" && r.mode !== "suggest") {
    context += "\n\n" + hypotheses
  }
  return { system, context, user: userMessage(context, r.prompt) }
}
