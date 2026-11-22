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

type AnchorField =
  "from" | "to" | "series" | "row" | "column" | "category" | "id" | "quote"

const fieldOrder: readonly AnchorField[] = [
  "from",
  "to",
  "series",
  "row",
  "column",
  "category",
  "id",
  "quote",
]

const kindRules: Record<
  AnchorKind,
  { required: readonly AnchorField[]; optional?: readonly AnchorField[] }
> = {
  time: { required: ["from"] },
  range: { required: ["from", "to"] },
  series: { required: ["series"] },
  point: { required: ["series", "from"] },
  row: { required: ["row"], optional: ["quote"] },
  column: { required: ["column"] },
  cell: { required: ["row", "column"] },
  bar: { required: ["category"], optional: ["series"] },
  segment: { required: ["category", "series"] },
  heat_cell: { required: ["from", "category"] },
  slice: { required: ["category"] },
  value: { required: [], optional: ["series"] },
  log_line: { required: ["id"], optional: ["from", "quote", "series"] },
  span: { required: ["id"], optional: ["quote"] },
  element: { required: ["id"] },
  text: { required: ["quote"] },
  title: { required: [] },
  variable: { required: ["id"] },
  position: { required: [] },
}

export const maxLabels = 16
export const maxText = 200
const maxQuote = 280
export const maxEncoded = 1536

const wireFields = new Set([
  "kind",
  "pos",
  "from",
  "to",
  "series",
  "row",
  "column",
  "category",
  "id",
  "quote",
  "shown",
  "window",
])

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function runeCount(text: string): number {
  return Array.from(text).length
}

function isAnchorKind(value: unknown): value is AnchorKind {
  return (anchorKinds as readonly unknown[]).includes(value)
}

function isPresent(anchor: Anchor, field: AnchorField): boolean {
  const value = anchor[field]
  if (typeof value === "string") return value !== ""
  if (value === undefined) return false
  return typeof value === "object" && Object.keys(value).length > 0
}

function instantOf(value: string | undefined): number {
  return value === undefined ? Number.NaN : Date.parse(value)
}

function labelProblem(
  name: string,
  labels: Record<string, string> | undefined
): string | null {
  if (!labels) return null
  const keys = Object.keys(labels)
  if (keys.length > maxLabels) {
    return `${name} has more than ${maxLabels} labels`
  }
  if (keys.includes("")) return `${name} has an empty label name`
  const tooLong = keys.some(
    (key) => runeCount(key) > maxText || runeCount(labels[key]) > maxText
  )
  return tooLong ? `${name} label is longer than ${maxText} characters` : null
}

function sizeProblem(anchor: Anchor): string | null {
  const labels =
    labelProblem("series", anchor.series) ?? labelProblem("row", anchor.row)
  if (labels) return labels
  const texts: [string, string | undefined, number][] = [
    ["column", anchor.column, maxText],
    ["category", anchor.category, maxText],
    ["id", anchor.id, maxText],
    ["quote", anchor.quote, maxQuote],
    ["shown", anchor.shown, maxText],
  ]
  for (const [name, value, limit] of texts) {
    if (value !== undefined && runeCount(value) > limit) {
      return `${name} is longer than ${limit} characters`
    }
  }
  const { window } = anchor
  if (window) {
    if (!window.from || !window.to) return "window needs from and to"
    if (runeCount(window.from) > maxText || runeCount(window.to) > maxText) {
      return `window is longer than ${maxText} characters`
    }
  }
  return null
}

function problemOf(anchor: Anchor): string | null {
  const rule = kindRules[anchor.kind]
  const { x, y } = anchor.pos
  const inUnit = (v: number) => Number.isFinite(v) && v >= 0 && v <= 1
  if (!inUnit(x) || !inUnit(y)) return "pos: x and y must be between 0 and 1"
  for (const field of rule.required) {
    if (!isPresent(anchor, field)) return `${anchor.kind} needs ${field}`
  }
  for (const field of fieldOrder) {
    const allowed =
      rule.required.includes(field) || rule.optional?.includes(field)
    if (isPresent(anchor, field) && !allowed) {
      return `${anchor.kind} does not take ${field}`
    }
  }
  if (
    anchor.kind === "range" &&
    instantOf(anchor.to) < instantOf(anchor.from)
  ) {
    return "range ends before it starts"
  }
  return sizeProblem(anchor) ?? encodedProblem(anchor)
}

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

function encodedProblem(anchor: Anchor): string | null {
  const bytes = encodedBytes(anchor)
  return bytes > maxEncoded
    ? `${bytes} bytes once encoded, over the limit of ${maxEncoded} bytes; send fewer or shorter labels`
    : null
}

function stringField(
  raw: Record<string, unknown>,
  name: string
): string | undefined {
  const value = raw[name]
  if (value === undefined) return undefined
  if (typeof value !== "string") throw new Error(`${name} must be a string`)
  return value
}

function timeField(
  raw: Record<string, unknown>,
  name: string
): string | undefined {
  const value = stringField(raw, name)
  if (value !== undefined && Number.isNaN(Date.parse(value))) {
    throw new Error(`${name} is not a time`)
  }
  return value
}

function labelsField(
  raw: Record<string, unknown>,
  name: string
): Record<string, string> | undefined {
  const value = raw[name]
  if (value === undefined) return undefined
  if (!isRecord(value)) throw new Error(`${name} must be an object`)
  const labels: Record<string, string> = {}
  for (const [key, label] of Object.entries(value)) {
    if (typeof label !== "string") {
      throw new Error(`${name} labels must be strings`)
    }
    labels[key] = label
  }
  return labels
}

function posField(raw: Record<string, unknown>): AnchorPos {
  const pos = raw.pos
  if (pos === undefined || pos === null) throw new Error("pos is required")
  if (
    !isRecord(pos) ||
    typeof pos.x !== "number" ||
    typeof pos.y !== "number" ||
    Object.keys(pos).some((key) => key !== "x" && key !== "y")
  ) {
    throw new Error("pos must be an object with numbers x and y")
  }
  return { x: pos.x, y: pos.y }
}

function windowField(raw: Record<string, unknown>): AnchorWindow | undefined {
  const window = raw.window
  if (window === undefined) return undefined
  if (
    !isRecord(window) ||
    typeof window.from !== "string" ||
    typeof window.to !== "string" ||
    Object.keys(window).some((key) => key !== "from" && key !== "to")
  ) {
    throw new Error("window must be an object with strings from and to")
  }
  return { from: window.from, to: window.to }
}

function withoutUndefined<T extends object>(value: T): T {
  return Object.fromEntries(
    Object.entries(value).filter(([, v]) => v !== undefined)
  ) as T
}

function shapeOf(value: unknown): Anchor {
  if (!isRecord(value)) throw new Error("must be an object")
  const unknownField = Object.keys(value).find((key) => !wireFields.has(key))
  if (unknownField !== undefined) {
    throw new Error(`unknown field "${unknownField}"`)
  }
  const kind = value.kind ?? ""
  if (!isAnchorKind(kind)) throw new Error(`unknown kind "${String(kind)}"`)
  return withoutUndefined({
    kind,
    pos: posField(value),
    from: timeField(value, "from"),
    to: timeField(value, "to"),
    series: labelsField(value, "series"),
    row: labelsField(value, "row"),
    column: stringField(value, "column"),
    category: stringField(value, "category"),
    id: stringField(value, "id"),
    quote: stringField(value, "quote"),
    shown: stringField(value, "shown"),
    window: windowField(value),
  })
}

export function parseAnchor(value: unknown): Anchor {
  try {
    const anchor = shapeOf(value)
    const problem = problemOf(anchor)
    if (problem) throw new Error(problem)
    return anchor
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    throw new Error(`anchor: ${message}`, { cause: error })
  }
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

function describeTime(value: string): string {
  const iso = new Date(value).toISOString()
  const seconds = iso.slice(17, 19) === "00" ? "" : iso.slice(16, 19)
  return `${iso.slice(0, 10)} ${iso.slice(11, 16)}${seconds} UTC`
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

function subject(anchor: Anchor): string {
  const from = anchor.from ? describeTime(anchor.from) : ""
  const to = anchor.to ? describeTime(anchor.to) : ""
  const series = anchor.series ? labelPairs(anchor.series) : ""
  const row = anchor.row ? readableCells(anchor.row) : ""
  const quote = anchor.quote ?? ""
  switch (anchor.kind) {
    case "time":
      return `the moment ${from}`
    case "range":
      return `the range ${from} to ${to}`
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

export type AnchorParts = {
  subject: string
  shown: string | null
  window: string | null
}

export function anchorParts(anchor: Anchor): AnchorParts {
  return {
    subject: subject(anchor),
    shown: anchor.shown ? anchor.shown : null,
    window: anchor.window ? describeWindow(anchor.window) : null,
  }
}

export function describeAnchor(anchor: Anchor): string {
  const parts = anchorParts(anchor)
  const shown = parts.shown === null ? "" : ` (${parts.shown})`
  const window = parts.window === null ? "" : `, ${parts.window}`
  return `${parts.subject}${shown}${window}`
}
