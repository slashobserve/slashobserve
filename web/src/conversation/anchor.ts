export const anchorKinds = [
  "time",
  "range",
  "series",
  "point",
  "row",
  "column",
  "cell",
  "bar",
  "segment",
  "heat_cell",
  "slice",
  "value",
  "log_line",
  "span",
  "element",
  "text",
  "title",
  "variable",
  "position",
] as const

type AnchorKind = (typeof anchorKinds)[number]

type AnchorPos = { x: number; y: number }

type AnchorWindow = { from: string; to: string }

export type Anchor = {
  kind: AnchorKind
  pos: AnchorPos
  from?: string
  to?: string
  series?: Record<string, string>
  row?: Record<string, string>
  column?: string
  category?: string
  id?: string
  quote?: string
  shown?: string
  window?: AnchorWindow
}

export const maxLabels = 16
export const maxText = 200
export const maxEncoded = 1536

const goEscapedExtra: Record<string, number> = {
  "<": 5,
  ">": 5,
  "&": 5,
  "\u2028": 3,
  "\u2029": 3,
}

export function encodedBytes(anchor: object): number {
  const json = JSON.stringify(anchor)
  let extra = 0
  for (const char of json) extra += goEscapedExtra[char] ?? 0
  return new TextEncoder().encode(json).length + extra
}

const windowUnits: Record<string, string> = {
  s: "second",
  m: "minute",
  h: "hour",
  d: "day",
  w: "week",
}

function describeWindow(window: AnchorWindow): string {
  const match = /^now-([1-9][0-9]*)([smhdw])$/.exec(window.from)
  if (match && window.to === "now") {
    const unit = windowUnits[match[2]]
    return match[1] === "1" ? `last ${unit}` : `last ${match[1]} ${unit}s`
  }
  return `${window.from} to ${window.to}`
}

function clockOf(iso: string): string {
  const seconds = iso.slice(17, 19) === "00" ? "" : iso.slice(16, 19)
  return `${iso.slice(11, 16)}${seconds}`
}

type TimeStyle = {
  at: (value: string) => string
  span: (from: string, to: string) => string
}

function fullTime(value: string): string {
  const iso = new Date(value).toISOString()
  return `${iso.slice(0, 10)} ${clockOf(iso)} UTC`
}

const fullTimes: TimeStyle = {
  at: fullTime,
  span: (from, to) => `${fullTime(from)} to ${fullTime(to)}`,
}

const monthNames = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
]

function shortDay(iso: string): string {
  return `${Number(iso.slice(8, 10))} ${monthNames[Number(iso.slice(5, 7)) - 1]}`
}

const shortTimes: TimeStyle = {
  at: (value) => {
    const iso = new Date(value).toISOString()
    return `${shortDay(iso)} ${clockOf(iso)} UTC`
  },
  span: (from, to) => {
    const start = new Date(from).toISOString()
    const end = new Date(to).toISOString()
    const endDay =
      start.slice(0, 10) === end.slice(0, 10) ? "" : `${shortDay(end)} `
    return `${shortDay(start)} ${clockOf(start)} to ${endDay}${clockOf(end)} UTC`
  },
}

function sortedKeys(labels: Record<string, string>): string[] {
  return Object.keys(labels).sort()
}

const longId = /^[0-9a-f]{16,}$/i
const idPrefix = 8

function readableCells(labels: Record<string, string>): string {
  return sortedKeys(labels)
    .map((key) => labels[key])
    .filter((value) => value !== "" && value !== "Unset")
    .map((value) =>
      longId.test(value) ? `${value.slice(0, idPrefix)}\u2026` : value
    )
    .join(", ")
}

function inRow(row: string): string {
  return row === "" ? "" : ` in the row ${row}`
}

function labelPairs(labels: Record<string, string>): string {
  return sortedKeys(labels)
    .map((key) => `${key}=${labels[key]}`)
    .join(", ")
}

function ofSeries(anchor: Anchor): string {
  return anchor.series && Object.keys(anchor.series).length > 0
    ? ` of the series ${labelPairs(anchor.series)}`
    : ""
}

function looksLikeQuery(name: string): boolean {
  return /[({[]/.test(name)
}

function valueSeries(anchor: Anchor): Anchor {
  const names = anchor.series ? Object.keys(anchor.series) : []
  const [only] = names
  return names.length === 1 &&
    only === "name" &&
    looksLikeQuery(anchor.series?.name ?? "")
    ? { ...anchor, series: undefined }
    : anchor
}

function percent(value: number): string {
  return `${Math.round(value * 100)}%`
}

function subject(anchor: Anchor, times: TimeStyle): string {
  const from = anchor.from ? times.at(anchor.from) : ""
  const series = anchor.series ? labelPairs(anchor.series) : ""
  const row = anchor.row ? readableCells(anchor.row) : ""
  const quote = anchor.quote ?? ""
  switch (anchor.kind) {
    case "time":
      return `the moment ${from}`
    case "range":
      return `the range ${
        anchor.from && anchor.to ? times.span(anchor.from, anchor.to) : ""
      }`
    case "series":
      return `the series ${series}`
    case "point":
      return `the point at ${from} on the series ${series}`
    case "row":
      return [`the row ${row}`.trim(), quote].filter(Boolean).join(", ")
    case "column":
      return `the column ${anchor.column}`
    case "cell":
      return `the cell ${anchor.column}${inRow(row)}`
    case "bar":
      return `the bar ${anchor.category}${ofSeries(anchor)}`
    case "segment":
      return `the segment ${anchor.category}${ofSeries(anchor)}`
    case "heat_cell":
      return `the heat cell ${anchor.category} at ${from}`
    case "slice":
      return `the slice ${anchor.category}`
    case "value":
      return `the value${ofSeries(valueSeries(anchor))}`
    case "log_line":
      return [
        ["the log line", quote && `"${quote}"`, from && `at ${from}`]
          .filter(Boolean)
          .join(" "),
        series && `in the series ${series}`,
      ]
        .filter(Boolean)
        .join(", ")
    case "span":
      return [`the span ${anchor.id}`, quote].filter(Boolean).join(", ")
    case "element":
      return `the element ${anchor.id}`
    case "text":
      return `the text "${quote}"`
    case "title":
      return "the title"
    case "variable":
      return `the variable ${anchor.id}`
    case "position":
      return `a spot at ${percent(anchor.pos.x)}, ${percent(anchor.pos.y)}`
  }
}

type AnchorParts = {
  subject: string
  shown: string | null
  window: string | null
}

function partsOf(anchor: Anchor, times: TimeStyle): AnchorParts {
  return {
    subject: subject(anchor, times),
    shown: anchor.shown ? anchor.shown : null,
    window: anchor.window ? describeWindow(anchor.window) : null,
  }
}

const leadingArticle = /^(the|a) /

export function chipParts(anchor: Anchor): AnchorParts {
  const parts = partsOf(anchor, shortTimes)
  return { ...parts, subject: parts.subject.replace(leadingArticle, "") }
}

function sentence(parts: AnchorParts): string {
  const shown = parts.shown === null ? "" : ` (${parts.shown})`
  const window = parts.window === null ? "" : `, ${parts.window}`
  return `${parts.subject}${shown}${window}`
}

export function describeAnchor(anchor: Anchor): string {
  return sentence(partsOf(anchor, fullTimes))
}
