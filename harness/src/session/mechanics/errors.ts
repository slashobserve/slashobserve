import { CallError, type CallDetails } from "../types"
import { parseRetryAfter } from "./retry"
import { truncateUtf8 } from "./compact"

export const maxProviderMessageBytes = 1024
export const redactedKey = "[redacted]"

const plainToken = /^[A-Za-z0-9_.:-]{1,128}$/
const loneSurrogates =
  /[\ud800-\udbff](?![\udc00-\udfff])|(?<![\ud800-\udbff])[\udc00-\udfff]/g
const notPrintable = /[^\p{L}\p{M}\p{N}\p{P}\p{S} ]/gu

export type ErrorContext = {
  provider: string
  apiKey: string
  logProviderErrors: boolean
  requestIdHeader: string
  turn: AbortSignal
  call: AbortSignal
}

export function isPlainToken(text: string): boolean {
  return plainToken.test(text)
}

function plain(value: unknown): string {
  const text =
    typeof value === "number" && Number.isInteger(value) ? String(value) : value
  return typeof text === "string" && isPlainToken(text) ? text : ""
}

export function scrubMessage(message: string, apiKey: string): string {
  const keyless =
    apiKey === "" ? message : message.split(apiKey).join(redactedKey)
  const printable = keyless
    .replace(loneSurrogates, "")
    .replace(notPrintable, " ")
    .trim()
  return truncateUtf8(printable, maxProviderMessageBytes)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function parsedBody(error: Record<string, unknown>): unknown {
  if (error["data"] !== undefined) {
    return error["data"]
  }
  const body = error["responseBody"]
  if (typeof body !== "string") {
    return undefined
  }
  try {
    return JSON.parse(body)
  } catch {
    return undefined
  }
}

function header(headers: unknown, name: string): string | undefined {
  if (!isRecord(headers)) {
    return undefined
  }
  for (const [key, value] of Object.entries(headers)) {
    if (key.toLowerCase() === name && typeof value === "string") {
      return value
    }
  }
  return undefined
}

function statusOf(error: unknown): number | undefined {
  if (!isRecord(error)) {
    return undefined
  }
  const status = error["statusCode"]
  return typeof status === "number" && Number.isInteger(status) && status > 0
    ? status
    : undefined
}

function detailsOf(error: Record<string, unknown>, ctx: ErrorContext) {
  const body = parsedBody(error)
  const envelope =
    isRecord(body) && isRecord(body["error"]) ? body["error"] : {}
  const requestId = plain(header(error["responseHeaders"], ctx.requestIdHeader))
  const retryAfterMs = parseRetryAfter(
    header(error["responseHeaders"], "retry-after")
  )
  const details: CallDetails = {
    type: plain(envelope["type"]),
    code: plain(envelope["code"]),
    param: plain(envelope["param"]),
    requestId,
    retryAfterMs,
  }
  const message = envelope["message"]
  if (ctx.logProviderErrors && typeof message === "string") {
    details.providerMessage = scrubMessage(message, ctx.apiKey)
  }
  return details
}

export function toCallError(
  error: unknown,
  ctx: ErrorContext,
  idle: CallError | undefined
): unknown {
  if (idle !== undefined) {
    return idle
  }
  if (ctx.turn.aborted) {
    return ctx.turn.reason
  }
  const status = statusOf(error)
  if (status !== undefined && isRecord(error)) {
    const failure = status === 429 ? "rate_limited" : "provider_error"
    return new CallError(
      ctx.provider,
      failure,
      status,
      false,
      detailsOf(error, ctx)
    )
  }
  if (ctx.call.aborted) {
    return new CallError(ctx.provider, "call_timeout")
  }
  return new CallError(ctx.provider, "provider_error")
}
