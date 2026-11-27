import { blank, type Figure, S, scan } from "./figures"
import { blankIDs } from "./traceids"
import { withoutCitations } from "./render"

const versionBefore = new RegExp(`\\b(?:version|release|v)${S}*$`, "iu")
const portBefore = new RegExp(`\\bports?${S}*$|[A-Za-z0-9_\\]]:$`, "iu")
const statusBefore = new RegExp(
  `\\b(?:http|status|codes?|returned|returning|returns|answered)${S}*(?:[1-5]\\d\\d${S}*(?:,|and|or|/)${S}*)*$`,
  "iu"
)
const windowBefore = new RegExp(
  `\\b(?:last|past|every|each|within|step|steps|interval|resolution|scrape|rate)${S}+(?:of${S}+)?$`,
  "iu"
)
const windowAfter = new RegExp(
  `^${S}*(?:steps?|windows?|intervals?|resolution|buckets?|scrape)\\b`,
  "iu"
)
const quantileBefore = new RegExp(`\\b(?:quantile|percentile)${S}*$`, "iu")
const quantileAfter = new RegExp(`^${S}*(?:quantile|percentile)\\b`, "iu")
const rankBefore = new RegExp(`\\b(?:top|first|bottom)${S}*$`, "iu")
const yearBefore = new RegExp(`\\b(?:in|since|year|during|of)${S}*$`, "iu")

type Exemption = (f: Figure, before: string, after: string) => boolean

function whole(f: Figure): boolean {
  return f.step >= 1 && !/[.,]/.test(f.text)
}

export function isDuration(unit: string): boolean {
  return unit === "s" || unit === "ms"
}

function statusCode(f: Figure): boolean {
  return (
    (f.unit === "" || (f.unit === "s" && !f.text.includes(" "))) &&
    whole(f) &&
    f.value >= 100 &&
    f.value <= 599
  )
}

const exemptions: Exemption[] = [
  (_, before) => versionBefore.test(before),
  (f, before) => f.unit === "" && f.step >= 1 && portBefore.test(before),
  (f, before) => statusCode(f) && statusBefore.test(before),
  (f, before, after) =>
    isDuration(f.unit) &&
    (windowBefore.test(before) || windowAfter.test(after)),
  (f, before, after) =>
    f.unit === "" && (quantileBefore.test(before) || quantileAfter.test(after)),
  (f, before) => f.unit === "" && f.step >= 1 && rankBefore.test(before),
  (f, before) =>
    f.unit === "" &&
    whole(f) &&
    f.value >= 1990 &&
    f.value <= 2100 &&
    yearBefore.test(before),
]

export function uncitedFigures(text: string, question: string): Figure[] {
  const asked = question === "" ? [] : scan(blank(question))
  const blanked = blank(blankIDs(withoutCitations(text)))
  const out: Figure[] = []
  for (const f of scan(blanked)) {
    if (f.unit === "" && f.step >= 1 && f.value <= 10) {
      continue
    }
    const before = blanked.slice(0, f.start)
    const after = blanked.slice(f.end)
    if (
      asked.some((a) => a.value === f.value) ||
      exemptions.some((e) => e(f, before, after))
    ) {
      continue
    }
    out.push(f)
  }
  return out
}
