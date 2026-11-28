import type { components } from "@/api/schema.gen"
import { lowerFirst } from "@/lib/text"

type MergeConflict = components["schemas"]["MergeConflict"]
export type ApiIssue = { path: string; code: string; message: string }

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly issues: ApiIssue[]
  readonly latestVersion: number | undefined
  readonly retryAfterMs: number | undefined
  readonly ref: string | undefined
  readonly conflicts: MergeConflict[]
  readonly requestedAt: string | undefined

  constructor(
    status: number,
    code: string,
    message: string,
    issues: ApiIssue[] = [],
    latestVersion?: number,
    retryAfterMs?: number,
    ref?: string,
    conflicts: MergeConflict[] = [],
    requestedAt?: string
  ) {
    super(message)
    this.name = "ApiError"
    this.status = status
    this.code = code
    this.issues = issues
    this.latestVersion = latestVersion
    this.retryAfterMs = retryAfterMs
    this.ref = ref
    this.conflicts = conflicts
    this.requestedAt = requestedAt
  }
}

export function failedQueryText(error: ApiError): string {
  return error.ref === undefined
    ? error.message
    : `Query ${error.ref} failed: ${lowerFirst(error.message)}`
}

export const unreachableText =
  "Could not reach the server. Check your connection and try again."

export class NetworkError extends Error {
  constructor(message: string, options?: ErrorOptions) {
    super(message, options)
    this.name = "NetworkError"
  }
}

export function sentence(text: string): string {
  const trimmed = text.trimEnd()
  if (trimmed === "") return ""
  const capitalized = trimmed.charAt(0).toUpperCase() + trimmed.slice(1)
  return /[.!?]$/.test(capitalized) ? capitalized : `${capitalized}.`
}

export type ProblemTexts = {
  codes?: Readonly<Record<string, string | ((error: ApiError) => string)>>
  notFound?: string
  fallback?: string
}

export function problemText(
  error: unknown,
  {
    codes = {},
    notFound,
    fallback = "Something went wrong. Try again.",
  }: ProblemTexts = {}
): string {
  if (error instanceof NetworkError) return unreachableText
  if (!(error instanceof ApiError)) return fallback
  const coded = Object.hasOwn(codes, error.code) ? codes[error.code] : undefined
  if (coded !== undefined) {
    return typeof coded === "string" ? coded : coded(error)
  }
  if (error.status === 404 && notFound !== undefined) return notFound
  return sentence(error.message) || fallback
}

export function loadProblemText(error: unknown): string {
  return problemText(error)
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

const conflictKinds = new Set(["panel", "variable", "setting"])
const conflictReasons = new Set([
  "both_changed",
  "changed_and_removed",
  "invalid",
])

function readConflicts(value: unknown): MergeConflict[] {
  if (!Array.isArray(value)) return []
  const conflicts: MergeConflict[] = []
  for (const item of value) {
    if (
      isRecord(item) &&
      typeof item.kind === "string" &&
      conflictKinds.has(item.kind) &&
      typeof item.id === "string" &&
      typeof item.reason === "string" &&
      conflictReasons.has(item.reason) &&
      typeof item.message === "string"
    ) {
      conflicts.push({
        kind: item.kind as MergeConflict["kind"],
        id: item.id,
        reason: item.reason as MergeConflict["reason"],
        message: item.message,
      })
    }
  }
  return conflicts
}

function readRetryAfterMs(response: Response): number | undefined {
  const header = response.headers.get("Retry-After")
  if (header === null || header.trim() === "") return undefined
  const seconds = Number(header)
  if (!Number.isFinite(seconds) || seconds < 0) return undefined
  return seconds * 1000
}

const accessLevelNames: Record<string, string> = {
  view: "View",
  comment: "Comment",
  edit: "Edit",
  manage: "Manage",
}

function neededText(fields: Record<string, unknown>): string {
  const access = fields.needed_access
  if (typeof access === "string" && access in accessLevelNames) {
    const folder = fields.folder_title
    return typeof folder === "string" && folder !== ""
      ? `it needs ${accessLevelNames[access]} on ${folder}`
      : `it needs ${accessLevelNames[access]} on the folder`
  }
  const role = fields.needed_role
  if (typeof role === "string" && role !== "") {
    return `it needs the ${role.charAt(0).toUpperCase()}${role.slice(1)} role`
  }
  return ""
}

export function toApiError(response: Response, body: unknown): ApiError {
  const fields = isRecord(body) ? body : {}
  const code =
    typeof fields.code === "string" && fields.code !== ""
      ? fields.code
      : "unknown"
  const base =
    typeof fields.error === "string" && fields.error !== ""
      ? fields.error
      : `Request failed (${response.status})`
  const needed = code === "forbidden_role" ? neededText(fields) : ""
  const message = needed === "" ? base : `${base}; ${needed}`
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
    typeof fields.ref === "string" && fields.ref !== ""
      ? fields.ref
      : undefined,
    readConflicts(fields.conflicts),
    typeof fields.requested_at === "string" && fields.requested_at !== ""
      ? fields.requested_at
      : undefined
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
  if (
    !(error instanceof ApiError) ||
    !retryableStatuses.has(error.status) ||
    error.code === "query_timeout"
  ) {
    return false
  }
  return (
    error.retryAfterMs === undefined || error.retryAfterMs <= maxRetryDelayMs
  )
}

export async function resendOnceOffline<T>(
  clientId: string | undefined,
  send: () => Promise<T>
): Promise<T> {
  try {
    return await send()
  } catch (error) {
    if (clientId === undefined || !(error instanceof NetworkError)) throw error
    return send()
  }
}
