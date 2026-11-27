import type { Mode } from "./types"

export const maxRejectionsInRow = 3
export const maxLengthRejections = 5
export const maxRejectedSaves = 8
export const maxIdenticalReads = 3
export const maxStalledTurns = 2
export const maxLedgerTurns = 4
export const maxChangesPerPanel = 4
export const maxChurnedSavesInRow = 3
export const maxWritesPerAttempt = 8
export const maxUnchecked = 8
export const maxReasonsShown = 2
export const limitGrowth = 3
export const quietTurnsToNudge = 2

export const defaultInvestigationCalls = 30
export const defaultInvestigationCallsMax = 40
export const defaultChatCalls = 12
export const defaultSaveCalls = 6
export const defaultSuggestCalls = 12

export const codeInvalidRequest = "invalid_request"
export const codeFindingTooLong = "finding_too_long"
export const codeRestrictedAnswer = "restricted_answer"

export const stepReusedRead = "Reused the result of an identical call"
export const sentBackStep = "The answer was sent back for a fix: "

export const messageReplacement =
  "This save deleted a panel and added another, which is not progress. Keep the panels you have, save the next panel, or set finish: true when every planned panel is built."
export const messageReplayed =
  "This is the result of the same earlier call. A third identical call is refused."
export const messageSaveNext =
  "Save the next panel, or call save_finding if the request only asks a question, in your next call. From then on, reads run only in the same call as a save."
export const messageAnswerNext =
  "Save the first panel if the request asks for a dashboard, otherwise call save_finding with what you found, in your next call. From then on, reads run only in the same call as a save."
export const messageProposeNext =
  "Propose the panel in your next call. From then on, reads run only in the same call as a proposal."
export const messageFindingNext =
  "One more read at most, then call save_finding."
export const messageConclude =
  "Your ledger has a supported cause and nothing open. Call save_finding now with the evidence you read, and say what you did not read."
export const messageReminder =
  "You read for two turns without changing your hypotheses. Send the updated ledger with your next reads, or call save_finding."

export const faultNoEvidence =
  "the finding gives counts but no evidence panel: add 1 to 4 evidence panels whose queries you ran and that returned data, so the numbers can be checked; if no query returned data, send it again unchanged"
export const faultTitle =
  "the title names a cause, but the text calls it likely or unconfirmed: title the symptom you saw instead, such as the service and what failed, or send it again unchanged when you did confirm the cause"
export const faultNoMetrics =
  'the finding says a metric is missing, but no list_metrics call in this run set scope: a service, job or pod is a label value, so list metrics with scope set to its selector, such as {service="checkout"}, before saying so, or drop the claim'
export const faultNoDeploy =
  "the finding says there was no deploy, but this run never read list_annotations, where deploy markers live: read it with tag deploy before saying so, or drop the claim"
export const faultLedgerBehind =
  "the finding names a cause or ruled-out items, but every hypothesis in your ledger is still open: send the whole ledger again with note_hypotheses, marking what you supported or ruled out with the evidence you read, then save the finding again"

export const peopleNoEvidence = "it gave counts without an evidence panel"
export const peopleTitle =
  "its title named a cause that the text calls unconfirmed"
export const peopleNoMetrics =
  "it said a metric is missing without checking for it"
export const peopleNoDeploy =
  "it said there was no deploy without reading the deploy markers"
export const peopleUnclosed =
  "it concluded while a triage candidate was not accounted for"
export const peopleLedgerBehind =
  "it named a cause while every hypothesis was still open"
export const peopleUngrounded =
  "it named a figure, service or time that the run never read"

export type GuardName =
  | "save_now"
  | "finding_now"
  | "propose_now"
  | "repeated_read"
  | "repeated_save"
  | "too_many_writes"
  | "repeated_write"
  | "read_in_flight"

export type Rule = { message: string; refused: string; stopped: string }

const fixedRules: Record<
  Exclude<GuardName, "save_now" | "finding_now" | "propose_now">,
  Rule
> = {
  repeated_read: {
    message: `this exact call already ran ${maxIdenticalReads - 1} times; use its earlier result, change its input, or save`,
    refused: `the same read was asked ${maxIdenticalReads} times`,
    stopped: `Stopped: the same read was asked ${maxIdenticalReads} times`,
  },
  repeated_save: {
    message:
      "this save is identical to an earlier one and changes nothing; save the next panel, or set finish: true when every planned panel is built",
    refused: "the same save was sent again",
    stopped: "Stopped: the same save was sent again and again",
  },
  too_many_writes: {
    message: `this run already made ${maxWritesPerAttempt} changes, the most one run may; say in your answer what is left for the person to do`,
    refused: `${maxWritesPerAttempt} writes already ran`,
    stopped: `Stopped: the model kept writing after ${maxWritesPerAttempt} writes`,
  },
  read_in_flight: {
    message:
      "a read in this same call is still running, so this change was not made; make it in your next call, after you have the read's result",
    refused: "a read from the same call was still running",
    stopped:
      "Stopped: every change was refused because a read from the same call was still running",
  },
  repeated_write: {
    message:
      "this change is identical to an earlier one in this run and changes nothing; say what you did instead of repeating it",
    refused: "the same write was sent again",
    stopped: "Stopped: the same write was sent again and again",
  },
}

export function ruleOf(
  name: GuardName,
  calls: number,
  answering: boolean
): Rule {
  switch (name) {
    case "propose_now":
      return {
        message: `${calls} model calls without a proposal; only a proposal can run now. Propose the panel you have checked.`,
        refused: `${calls} model calls passed without a proposal`,
        stopped: `Stopped: the model kept reading after ${calls} calls without a proposal`,
      }
    case "finding_now":
      return {
        message: `${calls} model calls without a finding; only save_finding can run now. Save what you found and the evidence you checked, and say in it what you did not read.`,
        refused: `${calls} model calls passed without a finding`,
        stopped: `Stopped: the model kept reading after ${calls} calls without a finding`,
      }
    case "save_now": {
      const choice = answering
        ? "Save the first panel if the request asks for a dashboard, otherwise call save_finding with what you found"
        : "Save the panel you have checked, or call save_finding if the request only asks a question"
      return {
        message: `${calls} model calls without a save; only a save can run now. ${choice}, and say in it what you did not read; reads in the same call as a save still run.`,
        refused: `${calls} model calls passed without a save`,
        stopped: `Stopped: the model kept reading after ${calls} calls without a save`,
      }
    }
  }
  return fixedRules[name]
}

export function codeOf(name: GuardName): string {
  return name === "too_many_writes" ||
    name === "repeated_write" ||
    name === "read_in_flight"
    ? name
    : codeInvalidRequest
}

export function rejectedThing(mode: Mode): string {
  switch (mode) {
    case "suggest":
      return "proposals"
    case "investigate":
      return "findings"
  }
  return "saves"
}

export function sentBackToolStep(tool: string, reason: string): string {
  return `The ${tool} call was sent back for a fix: ${reason}`
}

export function peopleText(reasons: readonly string[]): string {
  if (reasons.length <= maxReasonsShown) {
    return reasons.join("; and ")
  }
  return `${reasons.slice(0, maxReasonsShown).join("; ")}; and ${reasons.length - maxReasonsShown} more`
}

const goEscapes: Record<string, string> = {
  "<": "\\u003c",
  ">": "\\u003e",
  "&": "\\u0026",
  "\u2028": "\\u2028",
  "\u2029": "\\u2029",
}

export function refusalContent(code: string, message: string): string {
  return JSON.stringify({ code, message }).replace(
    /[<>&\u2028\u2029]/g,
    (c) => goEscapes[c] ?? c
  )
}
