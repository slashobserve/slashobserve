import { log } from "./log"
import type { ClaimApi } from "./mcp"
import {
  ToolError,
  type ClaimAnswer,
  type OfferedRun,
  type Usage,
} from "./protocol"
import { type ClaimedRun, runLoop } from "./run"
import type { SessionFactory } from "./session/types"

export const claimWaitSeconds = 30
export const initialBackoffMs = 1_000
export const maxBackoffMs = 30_000
export const maxReleasesPerRun = 3
const noModelMessage = "claim without model settings"

export type Handback = { held: number; released: number }

export type ClaimOptions = {
  waitSeconds?: number
  handback?: Handback
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
  failed_calls: 0,
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

async function release(client: ClaimApi, run: OfferedRun): Promise<boolean> {
  try {
    await client.releaseRun({
      run_id: run.id,
      attempt: run.attempt,
      claim_epoch: run.claim_epoch,
    })
    return true
  } catch (error) {
    if (!(error instanceof ToolError)) {
      log("warn", "release of a run failed", { run_id: run.id })
    }
    return false
  }
}

async function failWithoutSettings(
  client: ClaimApi,
  run: OfferedRun
): Promise<void> {
  try {
    await client.finishRun({
      run_id: run.id,
      attempt: run.attempt,
      outcome: "failed",
      message: "harness received the run without model settings",
      usage: noUsage,
    })
  } catch (error) {
    if (!(error instanceof ToolError)) {
      log("warn", "finish of a run without model settings failed", {
        run_id: run.id,
      })
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
  const handback = options.handback ?? { held: 0, released: 0 }
  const releasesWithoutSettings = new Map<string, number>()
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
      const releases = releasesWithoutSettings.get(offered.id) ?? 0
      log("warn", noModelMessage, {
        run_id: offered.id,
        releases,
        backoff_ms: backoff,
      })
      if (releases < maxReleasesPerRun) {
        releasesWithoutSettings.set(offered.id, releases + 1)
        await release(client, offered)
      } else {
        releasesWithoutSettings.delete(offered.id)
        await failWithoutSettings(client, offered)
      }
      pool.give()
      await backOff()
      continue
    }
    backoff = start
    releasesWithoutSettings.clear()
    handback.held++
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
      .then(async () => {
        if (!signal.aborted) {
          handback.held--
        } else if (await release(client, offered)) {
          handback.held--
          handback.released++
        }
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
