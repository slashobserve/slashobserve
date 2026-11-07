import { kqlFromLucene, looksLikeLucene } from "./query-form"
import { parseKql, type KqlError } from "./kql"

export type QueryProblem = {
  message: string
  headline: string
  explanation: string
  start: number
  end: number
  line: number
  column: number
  caret: string
  fix: { text: string; label: string } | null
  luceneSwitch: boolean
  nothingRan: true
}

const applyFixLabel = "Apply the suggested fix"
const rewriteLabel = "Rewrite as KQL"
const singleMark = /"([^\w\s])"/g

function wordAt(text: string, start: number): string {
  return /^\S*/.exec(text.slice(start))?.[0] ?? ""
}

function locate(text: string, start: number): { line: number; column: number } {
  const before = text.slice(0, start)
  const line = before.split("\n").length
  return { line, column: start - before.lastIndexOf("\n") }
}

function unclosedGroup(text: string, before: number): number | null {
  if (text[before] === "(") return before
  const open: number[] = []
  let quoted = false
  for (let i = 0; i < before; i++) {
    const c = text[i]
    if (quoted) {
      if (c === "\\") i++
      else if (c === '"') quoted = false
    } else if (c === '"') {
      quoted = true
    } else if (c === "(") {
      open.push(i)
    } else if (c === ")") {
      open.pop()
    }
  }
  return open.length === 0 ? null : open[open.length - 1]
}

function closeGroupAtNextAnd(text: string, open: number): string {
  let depth = 0
  let quoted = false
  for (let i = open + 1; i < text.length; i++) {
    const c = text[i]
    if (quoted) {
      if (c === "\\") i++
      else if (c === '"') quoted = false
    } else if (c === '"') {
      quoted = true
    } else if (c === "(") {
      depth++
    } else if (c === ")") {
      depth--
    } else if (depth === 0 && /\s/.test(c)) {
      const word = /^\s+and(?=\s)/i.exec(text.slice(i))
      if (word !== null) {
        return text.slice(0, i) + ")" + text.slice(i)
      }
    }
  }
  return text.trimEnd() + ")"
}

function groupFix(text: string, open: number): string | null {
  const atNextAnd = closeGroupAtNextAnd(text, open)
  if (parseKql(atNextAnd).error === null) return atNextAnd
  const atEnd = text.trimEnd() + ")"
  return parseKql(atEnd).error === null ? atEnd : null
}

function describe(
  text: string,
  error: KqlError
): { headline: string; explanation: string } {
  const { message, start } = error
  const column = locate(text, start).column
  const token = wordAt(text, start)
  if (message === 'expected ")"') {
    const open = unclosedGroup(text, start)
    if (open !== null) {
      return {
        headline: `Missing a closing parenthesis for the group at column ${locate(text, open).column}`,
        explanation: `The group that starts at ${wordAt(text, open)} is never closed. KQL groups need a closing parenthesis before the next and.`,
      }
    }
  }
  if (message === 'unexpected ")"') {
    return {
      headline: `A closing parenthesis has nothing to close, column ${column}`,
      explanation:
        "This closing parenthesis has no opening parenthesis, or the group is empty. Remove it or add the opening one.",
    }
  }
  if (message.startsWith("expected a value after NOT")) {
    return {
      headline: `Nothing follows not, column ${column}`,
      explanation:
        "not negates the term after it, for example not log.level : debug.",
    }
  }
  if (message.startsWith("expected a value after")) {
    return {
      headline: `A value is missing, column ${column}`,
      explanation:
        'Write a value after the operator, for example log.level : error. Quote a value that has spaces, like message : "payment failed".',
    }
  }
  if (message === "nested queries are not supported") {
    return {
      headline: `Nested queries are not supported, column ${column}`,
      explanation:
        "KQL's field:{ ... } form for nested fields is not supported. Write the query as Query DSL instead.",
    }
  }
  if (message === "a field name cannot start with *") {
    return {
      headline: `A field name cannot start with *, column ${column}`,
      explanation:
        "Put the wildcard after a prefix, for example log.*, or search the value instead of the field name.",
    }
  }
  if (message === "quote a value that contains spaces") {
    return {
      headline: `Quote the value, column ${column}`,
      explanation:
        'A value with a space needs double quotes, for example message : "payment failed".',
    }
  }
  if (message === "unterminated string") {
    return {
      headline: `A quoted value is never closed, column ${column}`,
      explanation: "Add the closing double quote.",
    }
  }
  if (message === "kql: is a form prefix, not a query") {
    return {
      headline: `kql: belongs at the start of the query, column ${column}`,
      explanation:
        "kql: at the very start selects the KQL form. Inside a query, write the field name without it.",
    }
  }
  if (message === "query too complex") {
    return {
      headline: `The query is too large, column ${column}`,
      explanation:
        "KQL allows 20 levels of parentheses and 200 terms. Split the query or narrow it with a filter.",
    }
  }
  return {
    headline: `${message[0].toUpperCase()}${message.slice(1)}, column ${column}`,
    explanation:
      `Check the query at the marked column. ${token === "" ? "" : `The reader stopped at ${token}.`}`.trim(),
  }
}

function closeOpenGroup(text: string): string | null {
  const { error } = parseKql(text)
  if (error === null) return text
  if (error.message !== 'expected ")"') return null
  const open = unclosedGroup(text, error.start)
  return open === null ? null : groupFix(text, open)
}

function fixFor(text: string): { text: string; label: string } | null {
  const rewritten = looksLikeLucene(text) ? kqlFromLucene(text) : null
  const closed = closeOpenGroup(rewritten ?? text)
  if (closed !== null && closed !== text) {
    return {
      text: closed,
      label: rewritten === null ? applyFixLabel : rewriteLabel,
    }
  }
  return rewritten === null ? null : { text: rewritten, label: rewriteLabel }
}

export function problemOf(
  text: string,
  parse: ReturnType<typeof parseKql>,
  offset = 0
): QueryProblem | null {
  const { error } = parse
  if (error === null) return null
  const body = text.slice(offset)
  const { headline, explanation } = describe(body, error)
  const { line, column } = locate(body, error.start)
  const fix = fixFor(body)
  return {
    message: error.message,
    headline,
    explanation,
    start: offset + error.start,
    end: offset + error.end,
    line,
    column,
    caret: `${" ".repeat(column - 1)}^ ${error.message.replace(singleMark, "$1")} here`,
    fix:
      fix === null ? null : { ...fix, text: text.slice(0, offset) + fix.text },
    luceneSwitch: looksLikeLucene(body),
    nothingRan: true,
  }
}
