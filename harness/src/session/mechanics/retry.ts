import { CallError } from "../types"

export const maxAttempts = 4
export const baseBackoffMs = 1_000
export const maxBackoffMs = 30_000
export const maxRetryAfterMs = 60_000

const retryableStatuses = new Set([408, 500, 502, 503, 504, 529])

export type RetryEnv = {
  sleep: (ms: number, signal: AbortSignal) => Promise<void>
  random: () => number
  now: () => number
}

export const realEnv: RetryEnv = {
  sleep: (ms, signal) =>
    new Promise((resolve, reject) => {
      if (signal.aborted) {
        reject(signal.reason)
        return
      }
      const onAbort = (): void => {
        clearTimeout(timer)
        reject(signal.reason)
      }
      const timer = setTimeout(() => {
        signal.removeEventListener("abort", onAbort)
        resolve()
      }, ms)
      signal.addEventListener("abort", onAbort, { once: true })
    }),
  random: Math.random,
  now: Date.now,
}

export type RetryOptions = { deadline?: number; env?: RetryEnv }

export type Retried<T> = { value: T; attempts: number }

export function retryable(error: unknown): boolean {
  if (!(error instanceof CallError)) {
    return false
  }
  if (error.failure === "rate_limited" || error.idle) {
    return true
  }
  if (error.status === 0) {
    return error.failure === "provider_error"
  }
  return retryableStatuses.has(error.status)
}

export function retryWaitMs(
  retry: number,
  error: unknown,
  random: () => number
): number {
  const ceiling = Math.min(baseBackoffMs * 2 ** retry, maxBackoffMs)
  const half = Math.floor(ceiling / 2)
  const jittered = half + Math.floor(random() * (ceiling - half + 1))
  if (error instanceof CallError) {
    return Math.max(jittered, Math.min(error.retryAfterMs, maxRetryAfterMs))
  }
  return jittered
}

export function parseRetryAfter(header: string | undefined): number {
  if (header === undefined || !/^[+-]?\d+$/.test(header)) {
    return 0
  }
  const seconds = BigInt(header)
  if (seconds <= 0n || seconds > 9223372036854775807n) {
    return 0
  }
  return Math.min(Number(seconds), maxRetryAfterMs / 1_000) * 1_000
}

export async function retry<T>(
  attempt: (n: number) => Promise<T>,
  signal: AbortSignal,
  options: RetryOptions = {}
): Promise<Retried<T>> {
  const env = options.env ?? realEnv
  for (let n = 1; ; n++) {
    try {
      return { value: await attempt(n), attempts: n }
    } catch (error) {
      if (n >= maxAttempts || signal.aborted || !retryable(error)) {
        throw error
      }
      const wait = retryWaitMs(n - 1, error, env.random)
      if (
        options.deadline !== undefined &&
        env.now() + wait > options.deadline
      ) {
        throw error
      }
      try {
        await env.sleep(wait, signal)
      } catch {
        throw error
      }
    }
  }
}
