import {
  byteLength,
  compareBytes,
  fields,
  formatDuration,
  formatFloat,
  formatRFC3339,
  goQuote,
} from "../brief/gofmt"
import { eventLookback } from "./cluster"
import {
  type Candidate,
  hour,
  minute,
  type Report,
  type Row,
  rungs,
  type ServiceSource,
  type Skip,
  signalRequests,
} from "./types"

export const maxBlockBytes = 6 << 10

const blockOpen = "<triage>"
const blockClose = "</triage>"
const blockLead =
  "Computed by the server before your first call, with no model: a starting point, not evidence. Start from the candidates and test each with your own reads; every figure your answer states needs its own read, and a candidate is a lead, not a cause."

const maxTextBytes = 120
const maxServicesShown = 12

const keep = 0
const prioCandidate = 1
const prioGap = 2
const prioSignals = 3
const prioEvidence = 4
const prioServices = 5
const prioRates = 6
const prioPatterns = 7
const prioBackground = 8
const prioSkipped = 9

type Line = { prio: number; text: string }

export function stamp(ms: number): string {
  return formatRFC3339(Math.floor(ms / 1000))
}

export function short(ms: number): string {
  if (ms % hour === 0) {
    return `${ms / hour}h`
  }
  if (ms % minute === 0) {
    return `${ms / minute}m`
  }
  return formatDuration(ms * 1e6)
}

export function figure(v: number, unit: string): string {
  switch (unit) {
    case "ratio":
      return formatFloat(v * 100, "g", 3) + "%"
    case "":
      return formatFloat(v, "g", 3)
  }
  return formatFloat(v, "g", 3) + " " + unit
}

export function onsetText(t: number | undefined): string {
  return t === undefined ? "not found at the steps read" : stamp(t)
}

export function clipText(s: string): string {
  s = fields(s).join(" ")
  if (byteLength(s) <= maxTextBytes) {
    return s
  }
  const raw = Buffer.from(s, "utf8")
  let cut = maxTextBytes
  while (cut > 0 && ((raw[cut] as number) & 0xc0) === 0x80) {
    cut--
  }
  return raw.subarray(0, cut).toString("utf8") + "..."
}

function onSource(uid: string): string {
  return uid === "" ? "" : " on " + uid
}

function sourceOr(s: string, fallback: string): string {
  return s === "" ? fallback : s
}

function noCandidates(skipped: readonly Skip[]): string {
  const text =
    "Candidates: none. No service's errors or latency moved against its baseline on any window the sweep read."
  const signals: string[] = []
  for (const s of skipped) {
    for (const signal of s.signals) {
      if (!signals.includes(signal)) {
        signals.push(signal)
      }
    }
  }
  if (signals.length === 0) {
    return text
  }
  signals.sort(compareBytes)
  return `Candidates: none among the reads that completed, which is not an all-clear: the reads of ${signals.join(", ")} were skipped or failed (see Skipped), so those signals were not fully compared on every window.`
}

function windowsLine(r: Report): string {
  let text = `Windows: ${rungs.map(short).join(", ")}, each ending ${stamp(r.end)} and compared with the same length just before it.`
  if (r.question) {
    text += ` The question's window is ${stamp(r.question.from)} to ${stamp(r.question.to)}.`
  }
  return text
}

function serviceLine(s: ServiceSource): string {
  const names = s.services.slice(0, maxServicesShown)
  let text = `Services on ${s.source} (${s.type} ${s.signal}`
  if (s.field !== "") {
    text += ", in " + s.field
  }
  text += "): " + names.join(", ")
  const more = s.services.length - names.length
  if (more > 0) {
    text += ` and ${more} more`
  }
  if (s.services.length === 0) {
    text += "none seen"
  }
  return text
}

function moveText(r: Row): string {
  return `${r.signal} ${figure(r.window, r.unit)} from ${figure(r.baseline, r.unit)}`
}

function rateLines(rows: readonly Row[]): Line[] {
  const out: Line[] = []
  for (let i = 0; i < rows.length;) {
    const head = rows[i] as Row
    const parts = [moveText(head)]
    i++
    for (; i < rows.length && (rows[i] as Row).signal !== signalRequests; i++) {
      parts.push(moveText(rows[i] as Row))
    }
    out.push({
      prio: prioRates,
      text: `Last ${short(head.rung)} on ${head.source}, ${head.service}: ${parts.join(", ")}`,
    })
  }
  return out
}

function candidateLines(c: Candidate): Line[] {
  const parts = c.rows.map(
    (row) => `${moveText(row)} (${short(row.rung)}, ${row.source} ${row.basis})`
  )
  let head = `${c.id} ${c.service}: ${parts.join("; ")}; onset ${onsetText(c.onset)}`
  if (c.knockOn.length > 0) {
    head += "; knock-on, relaying its errors: " + c.knockOn.join(", ")
  }
  const out: Line[] = [{ prio: prioCandidate, text: head }]
  for (const o of c.origins) {
    let text = `  origin (${o.source}): ${o.service} ${clipText(o.operation)}`
    if (o.message !== "") {
      text += " " + goQuote(clipText(o.message))
    }
    text += `, ${o.traces} failing traces, example ${o.example}`
    if (o.relayedBy.length > 0) {
      text += ", relayed by " + o.relayedBy.join(", ")
    }
    out.push({ prio: prioEvidence, text })
  }
  for (const e of c.events) {
    let text = `  change (${sourceOr(e.source, "SlashObserve")}): ${e.kind} at ${stamp(e.at)}`
    if (e.service !== "") {
      text += " on " + e.service
    }
    if (e.text !== "") {
      text += ": " + clipText(e.text)
    }
    out.push({ prio: prioEvidence, text })
  }
  for (const p of c.patterns) {
    const state =
      p.baseline === 0
        ? `new, ${p.window} lines`
        : `grew to ${p.window} from ${p.baseline}`
    out.push({
      prio: prioPatterns,
      text: `  log pattern (${p.source}, ${sourceOr(p.level, "no level")}) ${state}: ${goQuote(clipText(p.text))}`,
    })
  }
  return out
}

function reportLines(r: Report): Line[] {
  const lines: Line[] = [
    { prio: keep, text: blockOpen },
    { prio: keep, text: blockLead },
    { prio: keep, text: windowsLine(r) },
  ]
  for (const s of r.services) {
    lines.push({ prio: prioServices, text: serviceLine(s) })
  }
  for (const s of r.signals) {
    lines.push({ prio: prioSignals, text: "Signal: " + s })
  }
  lines.push(...rateLines(r.rates))
  lines.push({
    prio: keep,
    text:
      r.candidates.length === 0
        ? noCandidates(r.skipped)
        : `Candidates, by onset (change events within ${short(eventLookback)} before an onset):`,
  })
  for (const c of r.candidates) {
    lines.push(...candidateLines(c))
  }
  for (const b of r.background) {
    lines.push({
      prio: prioBackground,
      text: `Background, not a candidate: ${b.service} ${b.signal} ${figure(b.window, b.unit)}, ${figure(b.baseline, b.unit)} before it (${short(b.rung)}, ${b.source}).`,
    })
  }
  for (const s of r.skipped) {
    lines.push({
      prio: s.signals.length > 0 ? prioGap : prioSkipped,
      text: `Skipped: ${s.step}${onSource(s.source)}: ${s.reason}.`,
    })
  }
  lines.push(
    {
      prio: keep,
      text: `Sweep: ${r.calls} reads in ${formatDuration(roundDuration(r.duration))}.`,
    },
    { prio: keep, text: blockClose }
  )
  return lines
}

export function roundDuration(ms: number): number {
  const nanos = Math.round(ms * 1e6)
  const m = 100e6
  const r = nanos % m
  const down = nanos - r
  return r + r < m ? down : down + m
}

function cutNote(n: number): string {
  return `Cut for size: ${n} lines, skipped steps first, then background, log patterns, per-service rates, service lists, evidence, signals, skipped rate reads and the last candidates.`
}

function size(lines: readonly Line[], cut: number): number {
  let n = 0
  for (const l of lines) {
    n += byteLength(l.text) + 1
  }
  if (cut > 0) {
    n += byteLength(cutNote(cut)) + 1
  }
  return n
}

function lowest(lines: readonly Line[]): number {
  let best = keep
  let at = -1
  lines.forEach((l, i) => {
    if (l.prio !== keep && l.prio >= best) {
      best = l.prio
      at = i
    }
  })
  return at
}

function fit(lines: readonly Line[], budget: number): string {
  const kept = [...lines]
  let cut = 0
  while (size(kept, cut) > budget) {
    const i = lowest(kept)
    if (i < 0) {
      break
    }
    kept.splice(i, 1)
    cut++
  }
  let out = ""
  kept.forEach((l, i) => {
    if (cut > 0 && i === kept.length - 1) {
      out += cutNote(cut) + "\n"
    }
    out += l.text
    if (i < kept.length - 1) {
      out += "\n"
    }
  })
  return out
}

export function render(r: Report, budget: number): string {
  return fit(reportLines(r), budget)
}
