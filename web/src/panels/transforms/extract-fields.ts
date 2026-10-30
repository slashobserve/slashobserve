import type { Transform } from "@/spec"

import {
  copyFrame,
  matchesName,
  rowCount,
  withFields,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type ExtractOptions = Extract<Transform, { type: "extractFields" }>
type Row = Map<string, unknown>
type Parser = (text: string) => Row

const numberLiteral = /^[+-]?(\d+\.?\d*|\.\d+)(e[+-]?\d+)?$/i
const pair = /([\w.-]+)\s*[=:]\s*("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|[^\s,]+)/g
const pathStep = /[^.[\]]+|\[(\d+)\]/g

function unquote(text: string): string {
  const quote = text[0]
  if ((quote !== '"' && quote !== "'") || text.at(-1) !== quote) return text
  return text.slice(1, -1).replace(/\\(.)/g, "$1")
}

function readPath(root: unknown, path: string): unknown {
  let current = root
  for (const step of path.matchAll(pathStep)) {
    if (current === null || typeof current !== "object") return undefined
    const key = step[1] ?? step[0]
    if (!Object.hasOwn(current, key)) return undefined
    current = (current as Record<string, unknown>)[key]
  }
  return current
}

function parseJson(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

function jsonParser({ paths }: ExtractOptions): (text: string) => Row | null {
  return (text) => {
    const parsed = parseJson(text)
    if (
      parsed === null ||
      typeof parsed !== "object" ||
      Array.isArray(parsed)
    ) {
      return null
    }
    if (paths === undefined) return new Map(Object.entries(parsed))
    return new Map(
      paths.map(({ path, alias }) => [alias ?? path, readPath(parsed, path)])
    )
  }
}

const kvpParser: Parser = (text) =>
  new Map(
    [...text.matchAll(pair)].map((match) => [match[1], unquote(match[2])])
  )

function delimiterParser({ delimiter = "," }: ExtractOptions): Parser {
  return (text) =>
    new Map(text.split(delimiter).map((part, i) => [String(i + 1), part]))
}

function regexParser({ regex }: ExtractOptions): Parser | null {
  if (regex === undefined) return null
  let pattern: RegExp
  try {
    pattern = new RegExp(regex)
  } catch {
    return null
  }
  return (text) => new Map(Object.entries(pattern.exec(text)?.groups ?? {}))
}

function parserFor(options: ExtractOptions): (text: string) => Row | null {
  switch (options.format ?? "auto") {
    case "json":
      return jsonParser(options)
    case "kvp":
      return kvpParser
    case "delimiter":
      return delimiterParser(options)
    case "regexp":
      return regexParser(options) ?? (() => null)
    default: {
      const json = jsonParser(options)
      return (text) => json(text) ?? kvpParser(text)
    }
  }
}

function plain(value: unknown): string | null {
  if (value === null || value === undefined) return null
  return typeof value === "object" ? JSON.stringify(value) : String(value)
}

function isNumeric(value: unknown): boolean {
  return typeof value === "number" || numberLiteral.test(String(value).trim())
}

function columnOf(name: string, values: unknown[]): PreparedField {
  const present = values.filter((value) => value !== null)
  if (present.length > 0 && present.every(isNumeric)) {
    return {
      name,
      type: "number",
      values: values.map((value) => (value === null ? null : Number(value))),
    }
  }
  return { name, type: "string", values }
}

function extractFrame(
  frame: PreparedFrame,
  options: ExtractOptions,
  parse: (text: string) => Row | null
): PreparedFrame {
  const source = frame.fields.find((field) =>
    matchesName(frame, field, options.field)
  )
  if (source === undefined) return copyFrame(frame)
  const rows = rowCount(frame)
  const names: string[] = []
  const parsed: (Row | null)[] = []
  for (let i = 0; i < rows; i++) {
    const cell = source.values[i]
    const row = typeof cell === "string" ? parse(cell) : null
    parsed.push(row)
    for (const name of row?.keys() ?? []) {
      if (!names.includes(name)) names.push(name)
    }
  }
  const kept = options.replace
    ? frame.fields.filter((field) => options.keepTime && field.type === "time")
    : frame.fields
  const taken = new Set(kept.map((field) => field.name))
  const added = names
    .filter((name) => !taken.has(name))
    .map((name) =>
      columnOf(
        name,
        parsed.map((row) => plain(row?.get(name)))
      )
    )
  return withFields(frame, [...kept, ...added])
}

export function extractFields(
  frames: PreparedFrame[],
  options: ExtractOptions
): PreparedFrame[] {
  const parse = parserFor(options)
  return frames.map((frame) => extractFrame(frame, options, parse))
}
