import { formatFloat, goQuote } from "../brief/gofmt"
import { S } from "./figures"
import { kinds, type Problem } from "./problems"
import type { Fact } from "./record"

export const renderTolerance = 0.01
export const unreadMarker = "[figure not read]"
export const sampledLead = "at least "

const placeholderSource = `\\{\\{${S}*(f\\d+)((?:${S}*:${S}*[A-Za-z0-9%]+)*)${S}*\\}\\}`

export function placeholder(): RegExp {
  return new RegExp(placeholderSource, "gu")
}

export type Citation = { text: string; id: string; options: string[] }

export function citations(text: string): Citation[] {
  return [...text.matchAll(placeholder())].map((m) => ({
    text: m[0],
    id: m[1] as string,
    options: (m[2] ?? "")
      .split(":")
      .map((o) => o.replace(/^[\t\n\f\r ]+|[\t\n\f\r ]+$/g, ""))
      .filter((o) => o !== ""),
  }))
}

export function withoutCitations(text: string): string {
  return text.replace(placeholder(), (s) => " ".repeat(s.length))
}

export class Ledger {
  constructor(readonly facts: readonly Fact[]) {}

  fact(id: string): Fact | undefined {
    return this.facts.find((f) => f.id === id)
  }

  queryOf(id: string): string {
    return this.fact(id)?.query ?? ""
  }

  render(text: string): { text: string; problems: Problem[] } {
    const problems: Problem[] = []
    const out = text.replace(placeholder(), (match) => {
      const c = citations(match)[0] as Citation
      const f = this.fact(c.id)
      if (!f) {
        problems.push({
          kind: kinds.fact,
          text: c.text,
          fix: `${c.id} is not a fact this run read, or it is no longer held: read the value again and cite the id the result gives`,
        })
        return unreadMarker
      }
      const shown = format(f, c.options)
      if ("error" in shown) {
        problems.push({ kind: kinds.fact, text: c.text, fix: shown.error })
        return (format(f, []) as { text: string }).text
      }
      return shown.text
    })
    return { text: out, problems }
  }
}

export function render(
  ledger: Ledger | undefined,
  text: string
): { text: string; problems: Problem[] } {
  return (ledger ?? new Ledger([])).render(text)
}

const durationScales: Record<string, number> = {
  us: 1e-6,
  ms: 1e-3,
  s: 1,
  min: 60,
}

const unitText: Record<string, string> = {
  us: " µs",
  ms: " ms",
  s: " s",
  min: " min",
  "%": "%",
  bytes: " bytes",
}

type Rendering = {
  value: number
  unit: string
  digits: number
  bare: boolean
  moved: boolean
}

export function significant(v: number, digits: number): number {
  return Number(formatFloat(v, "g", digits))
}

export function plain(v: number): string {
  return formatFloat(significant(v, 12), "f", -1)
}

export function relative(shown: number, value: number): number {
  if (value === 0) {
    return Math.abs(shown)
  }
  return Math.abs(shown - value) / Math.abs(value)
}

function describeKind(f: Fact): string {
  return f.unit ? `${f.kind} in ${f.unit}` : f.kind
}

const namedUnits: [RegExp, string][] = [
  [/(^|[^a-z0-9])(ms|millis|milliseconds)([^a-z0-9]|$)/iu, "ms"],
  [/(^|[^a-z0-9])(micros|microseconds)([^a-z0-9]|$)/iu, "us"],
  [/(^|[^a-z0-9])(sec|secs|seconds)([^a-z0-9]|$)/iu, "s"],
]

function namedUnit(f: Fact): string {
  let found = ""
  for (const text of [f.of, f.query ?? "", f.asked ?? ""]) {
    for (const [match, unit] of namedUnits) {
      if (!match.test(text)) {
        continue
      }
      if (found !== "" && found !== unit) {
        return ""
      }
      found = unit
    }
    if (found !== "") {
      return found
    }
  }
  return ""
}

function apply(r: Rendering, f: Fact, opt: string): string | undefined {
  if (opt === "bare") {
    r.bare = true
    return undefined
  }
  if (opt === "%") {
    if ((f.kind !== "ratio" && f.kind !== "number") || r.unit !== "") {
      return `${f.id} is a ${describeKind(f)}, so it cannot be shown as a percent`
    }
    r.value *= 100
    r.unit = "%"
    r.moved = true
    return undefined
  }
  if (Object.hasOwn(durationScales, opt)) {
    return convert(r, f, opt)
  }
  if (opt.length === 1 && opt >= "1" && opt <= "9") {
    r.digits = Number(opt)
    return undefined
  }
  return `${goQuote(opt)} is not an option for ${f.id}; use ms, s, us, min, %, bare or 1 to 9 significant figures`
}

function convert(r: Rendering, f: Fact, opt: string): string | undefined {
  let from = r.unit
  if (from === "" && f.kind === "number") {
    from = namedUnit(f)
    if (from === "") {
      return `${f.id} is a number whose unit the read did not name, so it cannot be converted to ${opt}; cite it as {{${f.id}}} and write the unit its query measures after it`
    }
  }
  const scale = Object.hasOwn(durationScales, from)
    ? durationScales[from]
    : undefined
  if (scale === undefined || (f.kind !== "duration" && f.kind !== "number")) {
    return `${f.id} is a ${describeKind(f)}, so it cannot be shown in ${opt}`
  }
  r.value = (r.value * scale) / (durationScales[opt] as number)
  r.unit = opt
  r.moved = true
  return undefined
}

export function format(
  f: Fact,
  options: readonly string[]
): { text: string } | { error: string } {
  if (f.kind === "trace_id") {
    if (options.length > 0) {
      return {
        error: `${f.id} is a trace id, which takes no options; cite it as {{${f.id}}}`,
      }
    }
    return { text: f.text }
  }
  const r: Rendering = {
    value: f.value ?? 0,
    unit: f.unit ?? "",
    digits: 0,
    bare: false,
    moved: false,
  }
  for (const opt of options) {
    const err = apply(r, f, opt)
    if (err !== undefined) {
      return { error: err }
    }
  }
  let text = f.text
  if (r.moved || r.digits > 0) {
    let shown = r.value
    if (r.digits > 0) {
      shown = significant(r.value, r.digits)
      const off = relative(shown, r.value)
      if (off > renderTolerance) {
        return {
          error: `{{${f.id}:${r.digits}}} would show ${plain(shown)}, ${formatFloat(off * 100, "f", 1)}% away from ${plain(r.value)}; give more significant figures`,
        }
      }
      if (f.sampled && shown > r.value) {
        return {
          error: `{{${f.id}:${r.digits}}} would show at least ${plain(shown)}, above the ${plain(r.value)} counted in a sample; give more significant figures`,
        }
      }
    }
    text = plain(shown)
  }
  if (!r.bare) {
    text += unitText[r.unit] ?? ""
  }
  if (f.sampled) {
    text = sampledLead + text
  }
  return { text }
}
