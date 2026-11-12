import { log } from "./log"
import type { ClaimApi } from "./mcp"
import { ToolError, type ClaimAnswer, type Usage } from "./protocol"
import { type ClaimedRun, runLoop } from "./run"
import type { SessionFactory } from "./session/types"

export const claimWaitSeconds = 30
export const initialBackoffMs = 1_000
export const maxBackoffMs = 30_000
const noModelMessage = "claim without model settings"

export type ClaimOptions = {
  waitSeconds?: number
  initialBackoffMs?: number
  maxBackoffMs?: number
  sleep?: (ms: number, signal: AbortSignal) => Promise<void>
  run?: (
    claim: ClaimApi,
    claimed: ClaimedRun,
    factory: SessionFactory,
    signal: AbortSignal
  ) => Promise<void>
}

const noUsage: Usage = {
  provider: "none",
  model: "none",
  input_tokens: 0,
  output_tokens: 0,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  model_calls: 0,
  retries: 0,
  compactions: 0,
  renewals: 0,
  model_ms: 0,
  max_save_output_tokens: 0,
}

function sleepFor(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) {
      resolve()
      return
    }
    const onAbort = (): void => {
      clearTimeout(timer)
      resolve()
    }
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", onAbort)
      resolve()
    }, ms)
    signal.addEventListener("abort", onAbort, { once: true })
  })
}

class Slots {
  private free: number
  private waiting: Array<() => void> = []

  constructor(count: number) {
    this.free = count
  }

  async take(signal: AbortSignal): Promise<boolean> {
    while (this.free === 0) {
      if (signal.aborted) {
        return false
      }
      await new Promise<void>((resolve) => {
        const wake = (): void => {
          signal.removeEventListener("abort", wake)
          resolve()
        }
        signal.addEventListener("abort", wake, { once: true })
        this.waiting.push(wake)
      })
    }
    if (signal.aborted) {
      return false
    }
    this.free--
    return true
  }

  give(): void {
    this.free++
    this.waiting.shift()?.()
  }
}

async function release(
  client: ClaimApi,
  answer: ClaimAnswer,
  signal: AbortSignal
): Promise<void> {
  if (answer.run == null) {
    return
  }
  try {
    await client.finishRun(
      {
        run_id: answer.run.id,
        attempt: answer.run.attempt,
        outcome: "failed",
        message: "observe-agent received the run without model settings",
        usage: noUsage,
      },
      signal
    )
  } catch (error) {
    if (!(error instanceof ToolError) && !signal.aborted) {
      log("warn", "release of a run failed", { run_id: answer.run.id })
    }
  }
}

export async function claimLoop(
  client: ClaimApi,
  slots: number,
  factory: SessionFactory,
  signal: AbortSignal,
  options: ClaimOptions = {}
): Promise<void> {
  const sleep = options.sleep ?? sleepFor
  const start = options.initialBackoffMs ?? initialBackoffMs
  const ceiling = options.maxBackoffMs ?? maxBackoffMs
  const run = options.run ?? runLoop
  const pool = new Slots(slots)
  const held = new Set<Promise<void>>()
  let backoff = start

  while (await pool.take(signal)) {
    let answer: ClaimAnswer
    try {
      answer = await client.claimRun(
        options.waitSeconds ?? claimWaitSeconds,
        signal
      )
      backoff = start
    } catch (error) {
      pool.give()
      if (signal.aborted) {
        break
      }
      log("warn", "claim_run failed", {
        backoff_ms: backoff,
        error: error instanceof Error ? error.name : "unknown",
      })
      await sleep(backoff, signal)
      backoff = Math.min(backoff * 2, ceiling)
      continue
    }
    const { run: offered, session, model } = answer
    if (offered == null || session == null || model == null) {
      if (offered != null) {
        log("warn", noModelMessage, { run_id: offered.id })
        await release(client, answer, signal)
      }
      pool.give()
      if ((answer.retry_after_seconds ?? 0) > 0) {
        await sleep((answer.retry_after_seconds ?? 0) * 1000, signal)
      }
      continue
    }
    const task: Promise<void> = run(
      client,
      { run: offered, session, model },
      factory,
      signal
    )
      .catch((error: unknown) => {
        log("error", "run loop crashed", {
          run_id: offered.id,
          error: error instanceof Error ? error.name : "unknown",
        })
      })
      .finally(() => {
        held.delete(task)
        pool.give()
      })
    held.add(task)
    if ((answer.retry_after_seconds ?? 0) > 0) {
      await sleep((answer.retry_after_seconds ?? 0) * 1000, signal)
    }
  }
  await Promise.allSettled(held)
}
