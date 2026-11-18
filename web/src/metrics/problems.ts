import { ApiError } from "@/api/errors"
import { lineAndColumn } from "@/query-editor/line-column"
import type { LintResult } from "@/query-editor/server-lint"

export type SyntaxProblem = {
  message: string
  line: number
  column: number
  source: string
  caret: string
}

export function syntaxProblem(
  lint: LintResult | null,
  text: string
): SyntaxProblem | null {
  if (lint === null || lint.status !== "ok" || lint.query !== text) return null
  const item = lint.items.find((candidate) => candidate.severity === "error")
  if (item === undefined) return null
  const { line, column } = lineAndColumn(text, item.from)
  const source = text.split("\n")[line - 1] ?? ""
  return {
    message: item.message,
    line,
    column,
    source,
    caret: `${" ".repeat(column - 1)}^`,
  }
}

const limitHint =
  "The query returned more than the server allows. Add label matchers, aggregate with sum by (label), or narrow the time range."
const timeoutHint =
  "The query took too long. Narrow the time range or aggregate it, for example with sum by (label)."

export function failureHint(error: unknown): string | null {
  if (!(error instanceof ApiError)) return null
  if (error.code === "limit_exceeded") return limitHint
  if (error.code === "query_timeout") return timeoutHint
  return null
}

export function truncationNote(truncated: boolean): string | null {
  return truncated
    ? "The table stopped at the server's row limit. Add label matchers or aggregate to see the rest."
    : null
}
