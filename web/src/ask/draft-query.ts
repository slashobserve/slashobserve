import { api } from "@/api/client"
import { ApiError, NetworkError, unwrap } from "@/api/errors"
import type { components } from "@/api/schema.gen"
import { offlineText } from "@/logs/explore"

type DraftQuery = components["schemas"]["DraftQuery"]

type TraceFilters = components["schemas"]["TraceFilters"]

export type DraftSignal = "logs" | "traces"

export type DraftAnswer = {
  query: string
  language: DraftQuery["language"]
  traceFilters: TraceFilters | null
  explanation: string
  warnings: string[]
}

export type DraftFailure =
  | { kind: "cannot_translate"; reason: string }
  | { kind: "error"; text: string }
  | { kind: "aborted" }

export function draftQueryRequest(
  uid: string,
  {
    question,
    from,
    to,
    signal,
  }: { question: string; from: string; to: string; signal: DraftSignal },
  abortSignal?: AbortSignal
): Promise<DraftAnswer> {
  return api
    .POST("/datasources/{uid}/draft-query", {
      params: { path: { uid } },
      body: { question, from, to, signal },
      signal: abortSignal,
    })
    .then(unwrap)
    .then((draft) => ({
      query: draft.query,
      language: draft.language,
      traceFilters: draft.trace_filters,
      explanation: draft.explanation,
      warnings: draft.warnings,
    }))
}

export function draftFailure(error: unknown): DraftFailure {
  if (error instanceof DOMException && error.name === "AbortError") {
    return { kind: "aborted" }
  }
  if (error instanceof ApiError) {
    return error.code === "cannot_translate"
      ? { kind: "cannot_translate", reason: error.message }
      : { kind: "error", text: error.message }
  }
  if (error instanceof NetworkError) return { kind: "error", text: offlineText }
  return { kind: "error", text: "The question could not be sent. Try again." }
}
