import {
  fields,
  formatMinute,
  formatSecond,
  goQuote,
  hasSeconds,
  unixSeconds,
} from "./gofmt"

export type Anchor = {
  kind: string
  pos?: { x: number; y: number }
  from?: string
  to?: string
  series?: Record<string, string>
  row?: Record<string, string>
  column?: string
  category?: string
  id?: string
  quote?: string
  shown?: string
  window?: { from: string; to: string }
}

export type Pointer = {
  anchor?: Anchor | null
  block_id?: string | null
  observation_id?: string | null
  thread_ask?: boolean
}

const maxAnchorRunes = 500

const windowUnits: Record<string, string> = {
  s: "second",
  m: "minute",
  h: "hour",
  d: "day",
  w: "week",
}

function describeWindow(w: { from: string; to: string }): string {
  const m = /^now-([1-9][0-9]*)([smhdw])$/.exec(w.from)
  if (m && w.to === "now") {
    const unit = windowUnits[m[2] ?? ""] ?? ""
    return m[1] === "1" ? "last " + unit : `last ${m[1]} ${unit}s`
  }
  return w.from + " to " + w.to
}

function describeTime(text: string): string {
  const at = unixSeconds(text)
  return (hasSeconds(at) ? formatSecond(at) : formatMinute(at)) + " UTC"
}

function compareBytes(a: string, b: string): number {
  const x = Buffer.from(a, "utf8")
  const y = Buffer.from(b, "utf8")
  return Buffer.compare(x, y)
}

function sortedKeys(labels: Record<string, string>): string[] {
  return Object.keys(labels).sort(compareBytes)
}

function labelPairs(labels: Record<string, string> = {}): string {
  return sortedKeys(labels)
    .map((k) => k + "=" + labels[k])
    .join(", ")
}

const longID = /^[0-9a-fA-F]{16,}$/

function readableCells(labels: Record<string, string> = {}): string {
  const cells: string[] = []
  for (const k of sortedKeys(labels)) {
    const v = labels[k] ?? ""
    if (v === "" || v === "Unset") {
      continue
    }
    cells.push(longID.test(v) ? v.slice(0, 8) + "\u2026" : v)
  }
  return cells.join(", ")
}

function joinNonEmpty(...parts: string[]): string {
  return parts.filter((p) => p !== "").join(", ")
}

function percent(v: number): string {
  return String(Math.trunc(goRound(v * 100))) + "%"
}

function goRound(x: number): number {
  return x < 0 ? -Math.round(-x) : Math.round(x)
}

function fromText(a: Anchor): string {
  return a.from ? describeTime(a.from) : ""
}

function ofSeries(a: Anchor): string {
  return a.series && Object.keys(a.series).length > 0
    ? " of the series " + labelPairs(a.series)
    : ""
}

function inRow(row: string): string {
  return row === "" ? "" : " in the row " + row
}

function subject(a: Anchor): string {
  const quote = a.quote ?? ""
  switch (a.kind) {
    case "time":
      return "the moment " + fromText(a)
    case "range":
      return "the range " + fromText(a) + " to " + describeTime(a.to ?? "")
    case "series":
      return "the series " + labelPairs(a.series)
    case "point":
      return (
        "the point at " + fromText(a) + " on the series " + labelPairs(a.series)
      )
    case "row":
      return joinNonEmpty(("the row " + readableCells(a.row)).trim(), quote)
    case "column":
      return "the column " + (a.column ?? "")
    case "cell":
      return "the cell " + (a.column ?? "") + inRow(readableCells(a.row))
    case "bar":
      return "the bar " + (a.category ?? "") + ofSeries(a)
    case "segment":
      return "the segment " + (a.category ?? "") + ofSeries(a)
    case "heat_cell":
      return "the heat cell " + (a.category ?? "") + " at " + fromText(a)
    case "slice":
      return "the slice " + (a.category ?? "")
    case "value": {
      const series = a.series ?? {}
      const name = series["name"]
      if (
        name !== undefined &&
        Object.keys(series).length === 1 &&
        /[({[]/.test(name)
      ) {
        return "the value"
      }
      return "the value" + ofSeries(a)
    }
    case "log_line": {
      let line = "the log line"
      if (quote !== "") {
        line += ` "${quote}"`
      }
      const at = fromText(a)
      if (at !== "") {
        line += " at " + at
      }
      const series = a.series ?? {}
      const inSeries =
        Object.keys(series).length > 0
          ? "in the series " + labelPairs(series)
          : ""
      return joinNonEmpty(line, inSeries)
    }
    case "span":
      return joinNonEmpty("the span " + (a.id ?? ""), quote)
    case "element":
      return "the element " + (a.id ?? "")
    case "text":
      return `the text "${quote}"`
    case "title":
      return "the title"
    case "variable":
      return "the variable " + (a.id ?? "")
    case "position":
      return (
        "a spot at " + percent(a.pos?.x ?? 0) + ", " + percent(a.pos?.y ?? 0)
      )
  }
  throw new Error("unknown anchor kind")
}

export function describeAnchor(a: Anchor): string {
  let out = subject(a)
  if (a.shown) {
    out += " (" + a.shown + ")"
  }
  if (a.window) {
    out += ", " + describeWindow(a.window)
  }
  return out
}

function tagSafe(s: string): string {
  return s.replace(/[&<>]/g, (c) =>
    c === "&" ? "&amp;" : c === "<" ? "&lt;" : "&gt;"
  )
}

function ofObservation(p: Pointer): string {
  return p.observation_id ? " of observation " + p.observation_id : ""
}

export function anchorNote(p: Pointer): string {
  if (!p.anchor) {
    if (!p.block_id) {
      if (!p.observation_id || !p.thread_ask) {
        return ""
      }
      return `The request was asked on observation ${p.observation_id}; 'this' means that page.`
    }
    return `The request is about the whole of block ${p.block_id}${ofObservation(p)}.`
  }
  let what = fields(describeAnchor(p.anchor)).join(" ")
  const runes = [...what]
  if (runes.length > maxAnchorRunes) {
    what = runes.slice(0, maxAnchorRunes).join("") + "..."
  }
  what = tagSafe(what)
  const where = p.block_id ? " on block " + p.block_id + ofObservation(p) : ""
  return `The request points at ${goQuote(what)}${where}.`
}
