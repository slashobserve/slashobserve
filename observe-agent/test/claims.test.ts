import { describe, expect, test } from "bun:test"
import { claimLoop } from "../src/claims"
import { configureLog } from "../src/log"
import type { ClaimAnswer } from "../src/protocol"
import { ToolError } from "../src/protocol"
import type { ClaimedRun } from "../src/run"
import { FakeClaim, claimed, factory } from "./harness"

configureLog("error")

function offer(id: string, extra: Partial<ClaimAnswer> = {}): ClaimAnswer {
  const run = claimed("p")
  return {
    run: { ...run.run, id },
    session: run.session,
    model: run.model,
    ...extra,
  }
}

type Step = ClaimAnswer | Error | "wait"

class Script extends FakeClaim {
  readonly waits: number[] = []
  private index = 0

  constructor(
    private readonly steps: Step[],
    private readonly stop: AbortController
  ) {
    super()
  }

  override async claimRun(waitSeconds: number): Promise<ClaimAnswer> {
    this.waits.push(waitSeconds)
    const step = this.steps[this.index++]
    if (step === undefined || step === "wait") {
      this.stop.abort()
      return { run: null }
    }
    if (step instanceof Error) {
      throw step
    }
    return step
  }
}

const instantSleep = (log: number[]) => async (ms: number) => {
  log.push(ms)
}

describe("claim loop", () => {
  test("claims with a 30 second wait", async () => {
    const stop = new AbortController()
    const client = new Script(["wait"], stop)
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep([]),
    })
    expect(client.waits).toEqual([30])
  })

  test("never runs more than slots runs at once and a freed slot claims again", async () => {
    const stop = new AbortController()
    const ids = ["a", "b", "c", "d", "e"]
    const client = new Script([...ids.map((id) => offer(id)), "wait"], stop)
    let active = 0
    let peak = 0
    const started: string[] = []
    const release = new Map<string, () => void>()
    const run = async (_c: unknown, run: ClaimedRun): Promise<void> => {
      active++
      peak = Math.max(peak, active)
      started.push(run.run.id)
      await new Promise<void>((resolve) => release.set(run.run.id, resolve))
      active--
    }
    const loop = claimLoop(client, 2, factory, stop.signal, {
      sleep: instantSleep([]),
      run,
    })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(started).toEqual(["a", "b"])
    expect(client.waits).toHaveLength(2)
    release.get("a")?.()
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(started).toEqual(["a", "b", "c"])
    for (const id of ["b", "c"]) {
      release.get(id)?.()
    }
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(started).toEqual(["a", "b", "c", "d", "e"])
    for (const id of ["d", "e"]) {
      release.get(id)?.()
    }
    await loop
    expect(peak).toBe(2)
  })

  test("one claim_run at a time", async () => {
    const stop = new AbortController()
    let inFlight = 0
    let peak = 0
    const client = new Script(["wait"], stop)
    client.claimRun = async () => {
      inFlight++
      peak = Math.max(peak, inFlight)
      await new Promise((resolve) => setTimeout(resolve, 10))
      inFlight--
      if (client.waits.push(30) > 3) {
        stop.abort()
      }
      return offer(`r${client.waits.length}`)
    }
    await claimLoop(client, 4, factory, stop.signal, {
      run: async () => {},
      sleep: instantSleep([]),
    })
    expect(peak).toBe(1)
  })

  test("honours retry_after_seconds", async () => {
    const stop = new AbortController()
    const sleeps: number[] = []
    const client = new Script(
      [
        { run: null, retry_after_seconds: 7 },
        offer("a", { retry_after_seconds: 3 }),
        "wait",
      ],
      stop
    )
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep(sleeps),
      run: async () => {},
    })
    expect(sleeps).toEqual([7000, 3000])
  })

  test("backs off from 1 second doubling to 30 seconds and resets on success", async () => {
    const stop = new AbortController()
    const sleeps: number[] = []
    const failure = new Error("connection refused")
    const steps: Step[] = [
      failure,
      failure,
      failure,
      failure,
      failure,
      failure,
      failure,
      { run: null },
      failure,
      "wait",
    ]
    const client = new Script(steps, stop)
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep(sleeps),
    })
    expect(sleeps).toEqual([1000, 2000, 4000, 8000, 16000, 30000, 30000, 1000])
  })

  test("a tool error from claim_run backs off too", async () => {
    const stop = new AbortController()
    const sleeps: number[] = []
    const client = new Script(
      [new ToolError("not_a_worker", "no"), "wait"],
      stop
    )
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep(sleeps),
    })
    expect(sleeps).toEqual([1000])
  })

  test("a claim without model settings is released by finish_run failed with no code", async () => {
    const stop = new AbortController()
    const client = new Script([offer("a", { model: null }), "wait"], stop)
    let runs = 0
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep([]),
      run: async () => {
        runs++
      },
    })
    expect(runs).toBe(0)
    expect(client.finishes).toHaveLength(1)
    expect(client.finishes[0]).toMatchObject({
      run_id: "a",
      outcome: "failed",
    })
    expect(client.finishes[0]?.failure_code).toBeUndefined()
  })

  test("a failing release does not stop the loop", async () => {
    const stop = new AbortController()
    const client = new Script([offer("a", { model: null }), "wait"], stop)
    client.finishRun = async () => {
      throw new Error("down")
    }
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep([]),
    })
    expect(client.waits).toHaveLength(2)
  })

  test("a crashing run frees its slot", async () => {
    const stop = new AbortController()
    const client = new Script([offer("a"), offer("b"), "wait"], stop)
    const seen: string[] = []
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep([]),
      run: async (_c, run) => {
        seen.push(run.run.id)
        throw new Error("boom")
      },
    })
    expect(seen).toEqual(["a", "b"])
  })

  test("shutdown aborts held runs and waits for them", async () => {
    const stop = new AbortController()
    const client = new Script([offer("a")], stop)
    let aborted = false
    client.claimRun = (async () => {
      client.claimRun = async () => {
        await new Promise((resolve) => setTimeout(resolve, 10))
        stop.abort()
        return { run: null }
      }
      return offer("a")
    }) as typeof client.claimRun
    await claimLoop(client, 2, factory, stop.signal, {
      sleep: instantSleep([]),
      run: async (_c, _run, _f, signal) => {
        await new Promise<void>((resolve) =>
          signal.addEventListener("abort", () => {
            aborted = true
            resolve()
          })
        )
      },
    })
    expect(aborted).toBe(true)
  })
})
