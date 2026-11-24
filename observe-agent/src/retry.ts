import { StreamableHTTPError } from "@modelcontextprotocol/sdk/client/streamableHttp.js"
import { ToolError } from "./protocol"

export type RetryPolicy = {
  waitsMs: readonly number[]
  budgetMs: number
}

const retryableClientStatuses = new Set([408, 429])

export const startRetry: RetryPolicy = {
  waitsMs: [250, 1_000, 2_000, 4_000],
  budgetMs: 30_000,
}

function isRefusal(error: unknown): boolean {
  if (error instanceof ToolError) {
    return true
  }
  return (
    error instanceof StreamableHTTPError &&
    error.code !== undefined &&
    error.code >= 400 &&
    error.code < 500 &&
    !retryableClientStatuses.has(error.code)
  )
}

function pause(ms: number, signal: AbortSignal): Promise<void> {
  if (ms <= 0 || signal.aborted) {
    return Promise.resolve()
  }
  return new Promise((resolve) => {
    const done = () => {
      clearTimeout(timer)
      signal.removeEventListener("abort", done)
      resolve()
    }
    const timer = setTimeout(done, ms)
    signal.addEventListener("abort", done, { once: true })
  })
}

export async function retryWithin<T>(
  attempt: (signal: AbortSignal) => Promise<T>,
  signal: AbortSignal,
  policy: RetryPolicy
): Promise<T> {
  const bounded = Number.isFinite(policy.budgetMs)
  const deadline = Date.now() + policy.budgetMs
  for (let tries = 0; ; tries += 1) {
    const perTry = bounded
      ? AbortSignal.any([
          signal,
          AbortSignal.timeout(Math.max(1, deadline - Date.now())),
        ])
      : signal
    try {
      return await attempt(perTry)
    } catch (error) {
      const wait = policy.waitsMs[tries]
      if (
        isRefusal(error) ||
        signal.aborted ||
        wait === undefined ||
        Date.now() + wait >= deadline
      ) {
        throw error
      }
      await pause(wait, signal)
    }
  }
}
