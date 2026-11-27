import * as text from "./text"

const space = "[\\t\\n\\f\\r ]"

function anyCase(word: string): string {
  return word.replace(/[a-z]/g, (c) => `[${c}${c.toUpperCase()}]`)
}

const countUnits = [
  "%",
  ...[
    "ms",
    "s",
    "sec",
    "seconds?",
    "rps",
    "req/s",
    "errors?",
    "failures?",
    "spans?",
    "requests?",
    "calls?",
    "traces?",
    "lines?",
    "logs?",
    "rows?",
    "series",
    "services?",
    "pods?",
  ].map((unit) => `${anyCase(unit)}\\b`),
]

export const countClaim = new RegExp(
  `\\b\\d[\\d,]*(?:\\.\\d+)?(?:[kM]\\b|[xX]\\b|${space}?(?:${countUnits.join("|")}))`
)
const causeTitle =
  /\b(?:caused by|causes|due to|because of|tied to|linked to|driven by|blamed on)\b/i
const hedgedText =
  /\b(?:most likely|probably|possibly|unconfirmed|not confirmed|(?:could not|cannot|can't) confirm|could not be confirmed|appears to|seems to|may have|might have)\b/i
const sentenceEnd = new RegExp(`[.!?]+${space}+|\\n+`)
const wordToken = /[A-Za-z0-9]+/g
const wordPart = /[A-Z]?[a-z]+|[A-Z]+|[0-9]+/g

const noMetricsClaim =
  /\b(?:no|not any|without)\b[^.;:\n]{0,40}\bmetrics?\b|\bmetrics?\b[^.;:\n]{0,40}\b(?:do not|don't|does not|doesn't) exist|\bmetrics?\b[^.;:\n]{0,20}\b(?:are|is) missing/i
const noDeployClaim =
  /\b(?:no|not any|without)\b[^.;:\n]{0,30}\b(?:deploy(?:s|ment|ments)?|releases?|rollouts?)\b|\bdeploy(?:s|ment|ments)?\b[^.;:\n]{0,30}\b(?:not found|none|do not exist|does not exist)/i

const plainWords = new Set([
  "errors",
  "error",
  "failures",
  "failure",
  "failing",
  "service",
  "services",
  "requests",
  "latency",
  "with",
  "from",
  "that",
  "this",
  "after",
  "during",
])

export type Fault = { model: string; people: string; needsRead: boolean }

export type Evidence = {
  scopedMetrics: boolean
  annotations: boolean
  queried: boolean
}

export function noEvidence(): Evidence {
  return { scopedMetrics: false, annotations: false, queried: false }
}

export type Finding = {
  title: string
  markdown: string
  evidence: number
  conclusion: boolean
}

function record(input: string): Record<string, unknown> {
  try {
    const value: unknown = JSON.parse(input)
    return typeof value === "object" && value !== null && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : {}
  } catch {
    return {}
  }
}

function stringField(fields: Record<string, unknown>, name: string): string {
  const value = fields[name]
  return typeof value === "string" ? value : ""
}

function names(value: unknown): boolean {
  if (value === undefined || value === null) {
    return false
  }
  return !(
    typeof value === "object" &&
    !Array.isArray(value) &&
    Object.keys(value).length === 0
  )
}

export function findingOf(input: string): Finding {
  const fields = record(input)
  const list = (name: string) => {
    const value = fields[name]
    return Array.isArray(value) ? value.length : 0
  }
  return {
    title: stringField(fields, "title"),
    markdown: stringField(fields, "markdown"),
    evidence: list("evidence"),
    conclusion:
      list("ruled_out") > 0 || list("causes") > 0 || names(fields.root_cause),
  }
}

export function inputField(input: string, name: string): string {
  return stringField(record(input), name)
}

export function absenceFault(
  evidence: Evidence,
  finding: Finding
): Fault | undefined {
  if (!evidence.scopedMetrics && noMetricsClaim.test(finding.markdown)) {
    return {
      model: text.faultNoMetrics,
      people: text.peopleNoMetrics,
      needsRead: true,
    }
  }
  if (!evidence.annotations && noDeployClaim.test(finding.markdown)) {
    return {
      model: text.faultNoDeploy,
      people: text.peopleNoDeploy,
      needsRead: true,
    }
  }
  return undefined
}

export function evidenceFault(
  evidence: Evidence,
  finding: Finding
): Fault | undefined {
  if (
    evidence.queried &&
    finding.evidence === 0 &&
    countClaim.test(finding.markdown)
  ) {
    return {
      model: text.faultNoEvidence,
      people: text.peopleNoEvidence,
      needsRead: false,
    }
  }
  return undefined
}

export function titleFault(finding: Finding): Fault | undefined {
  if (doubtsTitleCause(finding.title, finding.markdown)) {
    return {
      model: text.faultTitle,
      people: text.peopleTitle,
      needsRead: false,
    }
  }
  return undefined
}

export function doubtsTitleCause(title: string, markdown: string): boolean {
  const found = causeTitle.exec(title)
  if (found === null) {
    return false
  }
  const cause = wordsOf(title.slice(found.index + found[0].length))
  if (cause.size === 0) {
    return false
  }
  return markdown
    .split(sentenceEnd)
    .some(
      (sentence) =>
        hedgedText.test(sentence) &&
        [...wordsOf(sentence)].some((word) => cause.has(word))
    )
}

function wordsOf(value: string): Set<string> {
  const words = new Set<string>()
  const add = (word: string) => {
    const lower = word.toLowerCase()
    if (lower.length >= 4 && !plainWords.has(lower)) {
      words.add(lower)
    }
  }
  for (const token of value.match(wordToken) ?? []) {
    add(token)
    for (const part of token.match(wordPart) ?? []) {
      add(part)
    }
  }
  return words
}
