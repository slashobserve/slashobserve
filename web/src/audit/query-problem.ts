import { ApiError, toApiError } from "@/api/errors"
import { byteRangeToIndices } from "@/query-editor/byte-offsets"

export type ByteSpan = { start: number; end: number }

export class AuditQueryError extends ApiError {
  readonly span: ByteSpan | null

  constructor(base: ApiError, span: ByteSpan | null) {
    super(base.status, base.code, base.message)
    this.span = span
  }
}

function byteSpanOf(body: unknown): ByteSpan | null {
  if (typeof body !== "object" || body === null) return null
  const { start, end } = body as { start?: unknown; end?: unknown }
  if (!Number.isInteger(start) || !Number.isInteger(end)) return null
  return { start: start as number, end: end as number }
}

export function auditError(response: Response, body: unknown): ApiError {
  const base = toApiError(response, body)
  return response.status === 422
    ? new AuditQueryError(base, byteSpanOf(body))
    : base
}

export function unwrapAudit<T>(result: {
  data?: T
  error?: unknown
  response: Response
}): T {
  if (!result.response.ok || result.error !== undefined) {
    throw auditError(result.response, result.error)
  }
  return result.data as T
}

export type MarkedSpan = { from: number; to: number }

export function markedSpan(text: string, span: ByteSpan): MarkedSpan {
  const { from, to } = byteRangeToIndices(text, span.start, span.end)
  if (from !== to) return { from, to }
  if (to < text.length) return { from, to: to + 1 }
  return { from: Math.max(0, from - 1), to }
}

export type MarkedQuery = { before: string; marked: string; after: string }

export function markedQuery(text: string, span: ByteSpan): MarkedQuery {
  const { from, to } = markedSpan(text, span)
  return {
    before: text.slice(0, from),
    marked: text.slice(from, to),
    after: text.slice(to),
  }
}

export function queryProblemOf(
  error: unknown,
  text: string
): { message: string; marked: MarkedQuery | null } | null {
  if (!(error instanceof AuditQueryError) || error.code !== "query_invalid") {
    return null
  }
  return {
    message: error.message,
    marked: error.span === null ? null : markedQuery(text, error.span),
  }
}

export function rejectionOf(
  error: unknown,
  text: string
): { message: string; marked: MarkedQuery | null } | null {
  if (!(error instanceof AuditQueryError)) return null
  return queryProblemOf(error, text) ?? { message: error.message, marked: null }
}
