import type { DraftAnswer } from "@/ask/draft-query"

import type { TraceFormSearch } from "./search"
import { parseDuration } from "./search-query"

function readableDuration(text: string | undefined): string | undefined {
  const trimmed = text?.trim()
  return trimmed && parseDuration(trimmed) !== null ? trimmed : undefined
}

function filled(text: string | undefined): string | undefined {
  const trimmed = text?.trim()
  return trimmed === "" ? undefined : trimmed
}

export function traceSearchFromAnswer(
  answer: Pick<DraftAnswer, "query" | "traceFilters">,
  type: string
): TraceFormSearch {
  const filters = answer.traceFilters ?? {}
  return {
    service: filled(filters.service),
    span: filled(filters.span),
    min: readableDuration(filters.min),
    max: readableDuration(filters.max),
    errors: filters.errors === true ? "1" : undefined,
    q: type === "tempo" ? filled(answer.query) : undefined,
  }
}
