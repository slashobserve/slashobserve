import { log } from "./log"
import type { ClaimApi } from "./mcp"
import { ToolError, type ClaimAnswer, type OfferedRun } from "./protocol"
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
  run: OfferedRun,
  signal: AbortSignal
): Promise<void> {
  try {
    await client.releaseRun({ run_id: run.id, attempt: run.attempt }, signal)
  } catch (error) {
    if (!(error instanceof ToolError) && !signal.aborted) {
      log("warn", "release of a run failed", { run_id: run.id })
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
  const backOff = async (): Promise<void> => {
    await sleep(backoff, signal)
    backoff = Math.min(backoff * 2, ceiling)
  }

  while (await pool.take(signal)) {
    let answer: ClaimAnswer
    try {
      answer = await client.claimRun(
        options.waitSeconds ?? claimWaitSeconds,
        signal
      )
    } catch (error) {
      pool.give()
      if (signal.aborted) {
        break
      }
      log("warn", "claim_run failed", {
        backoff_ms: backoff,
        error: error instanceof Error ? error.name : "unknown",
      })
      await backOff()
      continue
    }
    const { run: offered, session, model } = answer
    if (offered == null) {
      backoff = start
      pool.give()
      if ((answer.retry_after_seconds ?? 0) > 0) {
        await sleep((answer.retry_after_seconds ?? 0) * 1000, signal)
      }
      continue
    }
    if (session == null || model == null) {
      log("warn", noModelMessage, { run_id: offered.id, backoff_ms: backoff })
      await release(client, offered, signal)
      pool.give()
      await backOff()
      continue
    }
    backoff = start
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
