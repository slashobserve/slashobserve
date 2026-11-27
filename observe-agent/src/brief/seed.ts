import type { BriefReason } from "../protocol"

export type Reason = "" | BriefReason

const seedSentences: Record<BriefReason, string> = {
  restart: "The server restarted while you worked.",
  output_limit:
    "Your last answer was cut off at the output limit. Save one panel per call, and check only what that panel needs.",
  pending:
    "You answered without a tool call, but the build has fewer panels than you planned. Add the rest one per save, or resend planned_panels with the number you built, then finish.",
  call_timeout:
    "Your last call ran past the time limit for one model call. Keep each answer short: save one panel per call, and check only what that panel needs.",
  continue:
    "Your earlier run stopped before it finished, and the person asked you to continue it from the build above.",
  renewed:
    "Your session was renewed to keep it short; the build above holds every panel you saved.",
  conflict:
    "Someone saved a new version while you worked; your build was discarded. Make the change again on the dashboard above.",
  no_finding:
    "You answered without a tool call. Call save_finding now with your answer and the evidence you checked; your text alone is not saved. If you made something the person asked for, the finding names it and links it.",
  no_answer:
    "Your answer is not kept once the run ends. Call save_finding with it, or save a panel. If you made something the person asked for, the finding names it and links it.",
}

function sentenceOf(reason: Reason): string {
  return reason === "" ? "" : seedSentences[reason]
}

function heading(reason: Reason): string {
  return reason === "renewed" ||
    reason === "output_limit" ||
    reason === "call_timeout" ||
    reason === "pending"
    ? "Why your conversation was shortened: "
    : "Why this is a new session: "
}

export function resumeReason(request: {
  continues: boolean
  attempt: number
}): Reason {
  return request.continues && request.attempt === 1 ? "continue" : "restart"
}

export type Seed = {
  progress: { panels: number; planned_panels: number } | null
  reason: Reason
  note: string
}

function continueNote(note: string): string {
  return note === "" ? "" : "The person's note for this run: " + note + "\n"
}

export function buildSection(sd: Seed): string {
  const p = sd.progress ?? { panels: 0, planned_panels: 0 }
  let out = "\n<build>\n"
  out +=
    "You are building this dashboard. Your saves so far are in the build above; do not redo them.\n"
  out +=
    p.planned_panels > 0
      ? `Panels built: ${p.panels} of ${p.planned_panels}.\n`
      : `The build holds ${p.panels} panels.\n`
  const sentence = sentenceOf(sd.reason)
  if (sentence !== "") {
    out += heading(sd.reason) + sentence + "\n"
  }
  out += continueNote(sd.note)
  out +=
    "Continue with the next panel, one panel per save, and set finish: true on the last one. If nothing is missing, answer with one short sentence and no tool call.\n"
  return out + "</build>"
}

export function seedNote(sd: Seed): string {
  let out = ""
  const sentence = sentenceOf(sd.reason)
  if (sentence !== "") {
    out += "\n" + heading(sd.reason) + sentence
  }
  if (sd.note !== "") {
    out += "\n" + continueNote(sd.note)
  }
  return out
}
