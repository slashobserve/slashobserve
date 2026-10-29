import { lowerFirst } from "@/lib/text"

export type ApiIssue = { path: string; code: string; message: string }

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly issues: ApiIssue[]
  readonly latestVersion: number | undefined
  readonly retryAfterMs: number | undefined
  readonly ref: string | undefined

  constructor(
    status: number,
    code: string,
    message: string,
    issues: ApiIssue[] = [],
    latestVersion?: number,
    retryAfterMs?: number,
    ref?: string
  ) {
    super(message)
    this.name = "ApiError"
    this.status = status
    this.code = code
    this.issues = issues
    this.latestVersion = latestVersion
    this.retryAfterMs = retryAfterMs
    this.ref = ref
  }
}

export function failedQueryText(error: ApiError): string {
  return error.ref === undefined
    ? error.message
    : `Query ${error.ref} failed: ${lowerFirst(error.message)}`
}

export class NetworkError extends Error {
  constructor(message: string, options?: ErrorOptions) {
    super(message, options)
    this.name = "NetworkError"
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function readIssues(value: unknown): ApiIssue[] {
  if (!Array.isArray(value)) return []
  const issues: ApiIssue[] = []
  for (const item of value) {
    if (
      isRecord(item) &&
      typeof item.path === "string" &&
      typeof item.code === "string" &&
      typeof item.message === "string"
    ) {
      issues.push({ path: item.path, code: item.code, message: item.message })
    }
  }
  return issues
}

function readRetryAfterMs(response: Response): number | undefined {
  const header = response.headers.get("Retry-After")
  if (header === null || header.trim() === "") return undefined
  const seconds = Number(header)
  if (!Number.isFinite(seconds) || seconds < 0) return undefined
  return seconds * 1000
}

export function toApiError(response: Response, body: unknown): ApiError {
  const fields = isRecord(body) ? body : {}
  const message =
    typeof fields.error === "string" && fields.error !== ""
      ? fields.error
      : `Request failed (${response.status})`
  const code =
    typeof fields.code === "string" && fields.code !== ""
      ? fields.code
      : "unknown"
  const latestVersion =
    typeof fields.latest_version === "number" &&
    Number.isInteger(fields.latest_version)
      ? fields.latest_version
      : undefined
  return new ApiError(
    response.status,
    code,
    message,
    readIssues(fields.issues),
    latestVersion,
    readRetryAfterMs(response),
    typeof fields.ref === "string" && fields.ref !== "" ? fields.ref : undefined
  )
}

export function unwrap<T>(result: {
  data?: T
  error?: unknown
  response: Response
}): T {
  if (!result.response.ok || result.error !== undefined) {
    throw toApiError(result.response, result.error)
  }
  return result.data as T
}

const retryableStatuses = new Set([429, 502, 503, 504])

export const maxRetryDelayMs = 30_000

export function isRetryable(error: unknown): boolean {
  if (error instanceof NetworkError) return true
  if (!(error instanceof ApiError) || !retryableStatuses.has(error.status)) {
    return false
  }
  return (
    error.retryAfterMs === undefined || error.retryAfterMs <= maxRetryDelayMs
  )
}
