import { describe, expect, spyOn, test } from "bun:test"
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

const untilStopped = async (
  _claim: unknown,
  _run: ClaimedRun,
  _factory: unknown,
  signal: AbortSignal
): Promise<void> => {
  await new Promise<void>((resolve) =>
    signal.addEventListener("abort", () => resolve())
  )
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

  test("a claim without model settings goes back to the queue and is not failed", async () => {
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
    expect(client.releases).toEqual([{ run_id: "a", attempt: 1 }])
    expect(client.finishes).toHaveLength(0)
  })

  test("a release names the claim epoch of the claim it hands back", async () => {
    const stop = new AbortController()
    const claim = offer("a", { model: null })
    const client = new Script(
      [{ ...claim, run: { ...claim.run!, claim_epoch: 7 } }, "wait"],
      stop
    )
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep([]),
    })
    expect(client.releases).toEqual([
      { run_id: "a", attempt: 1, claim_epoch: 7 },
    ])
  })

  test("claims without model settings back off before the next claim and a usable answer resets it", async () => {
    const stop = new AbortController()
    const sleeps: number[] = []
    const unusable = offer("a", { model: null })
    const client = new Script(
      [
        unusable,
        unusable,
        unusable,
        { run: null },
        offer("b", { model: null }),
        "wait",
      ],
      stop
    )
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep(sleeps),
    })
    expect(client.releases).toHaveLength(4)
    expect(sleeps).toEqual([1000, 2000, 4000, 1000])
  })

  test("a run that comes back without model settings after three releases is failed with the reason", async () => {
    const stop = new AbortController()
    const unusable = offer("a", { model: null })
    const client = new Script(
      [unusable, unusable, unusable, unusable, "wait"],
      stop
    )
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep([]),
    })
    expect(client.releases.map((r) => r.run_id)).toEqual(["a", "a", "a"])
    expect(client.finishes).toHaveLength(1)
    expect(client.finishes[0]).toMatchObject({
      run_id: "a",
      attempt: 1,
      outcome: "failed",
      message: "harness received the run without model settings",
    })
    expect(client.finishes[0]?.failure_code).toBeUndefined()
  })

  test("each run without model settings has its own count of releases", async () => {
    const stop = new AbortController()
    const a = offer("a", { model: null })
    const b = offer("b", { model: null })
    const client = new Script([a, a, a, b, b, b, "wait"], stop)
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep([]),
    })
    expect(client.releases.map((r) => r.run_id)).toEqual([
      "a",
      "a",
      "a",
      "b",
      "b",
      "b",
    ])
    expect(client.finishes).toHaveLength(0)
  })

  test("a failed run starts a new count if it is ever offered again", async () => {
    const stop = new AbortController()
    const unusable = offer("a", { model: null })
    const client = new Script(
      [unusable, unusable, unusable, unusable, unusable, "wait"],
      stop
    )
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep([]),
    })
    expect(client.releases).toHaveLength(4)
    expect(client.finishes).toHaveLength(1)
  })

  test("a usable claim forgets the releases counted before it", async () => {
    const stop = new AbortController()
    const unusable = offer("a", { model: null })
    const client = new Script(
      [unusable, unusable, unusable, offer("c"), unusable, "wait"],
      stop
    )
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep([]),
      run: async () => {},
    })
    expect(client.releases).toHaveLength(4)
    expect(client.finishes).toHaveLength(0)
  })

  test("a failing release does not stop the loop", async () => {
    const stop = new AbortController()
    const client = new Script([offer("a", { model: null }), "wait"], stop)
    client.releaseRun = async () => {
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

  test("shutdown releases each held run after its loop has stopped, not tied to the stop signal", async () => {
    const stop = new AbortController()
    const client = new Script([offer("a"), offer("b"), "wait"], stop)
    const stopped: string[] = []
    const released: Array<{ run_id: string; afterItsLoop: boolean }> = []
    const signals: Array<AbortSignal | undefined> = []
    client.releaseRun = async (input, signal?: AbortSignal) => {
      released.push({
        run_id: input.run_id,
        afterItsLoop: stopped.includes(input.run_id),
      })
      signals.push(signal)
      return { state: "queued" }
    }
    await claimLoop(client, 3, factory, stop.signal, {
      sleep: instantSleep([]),
      run: async (_c, run, _f, signal) => {
        await new Promise<void>((resolve) =>
          signal.addEventListener("abort", () => resolve())
        )
        stopped.push(run.run.id)
      },
    })
    expect(released).toEqual([
      { run_id: "a", afterItsLoop: true },
      { run_id: "b", afterItsLoop: true },
    ])
    expect(signals).toEqual([undefined, undefined])
  })

  test("a run that ended before shutdown is not released", async () => {
    const stop = new AbortController()
    const client = new Script([offer("a"), "wait"], stop)
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep([]),
      run: async () => {},
    })
    expect(client.releases).toHaveLength(0)
  })

  test("a failing shutdown release is logged and its run is counted as still held", async () => {
    const stop = new AbortController()
    const client = new Script([offer("a"), offer("b"), "wait"], stop)
    client.releaseRun = async (input) => {
      if (input.run_id === "a") {
        throw new Error("down")
      }
      return { state: "queued" }
    }
    const handback = { held: 0, released: 0 }
    const lines: string[] = []
    const stderr = spyOn(process.stderr, "write").mockImplementation(
      (line) => lines.push(String(line)) > 0
    )
    configureLog("warn")
    try {
      await claimLoop(client, 3, factory, stop.signal, {
        sleep: instantSleep([]),
        handback,
        run: untilStopped,
      })
    } finally {
      configureLog("error")
      stderr.mockRestore()
    }
    expect(handback).toEqual({ held: 1, released: 1 })
    expect(lines.map((line) => JSON.parse(line))).toEqual([
      expect.objectContaining({
        level: "warn",
        msg: "release of a run failed",
        run_id: "a",
      }),
    ])
  })

  test("a run that ended before shutdown is neither held nor released in the count", async () => {
    const stop = new AbortController()
    const client = new Script([offer("a"), "wait"], stop)
    const handback = { held: 0, released: 0 }
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep([]),
      handback,
      run: async () => {},
    })
    expect(handback).toEqual({ held: 0, released: 0 })
  })

  test("a run whose loop has not stopped is counted as held", async () => {
    const stop = new AbortController()
    const client = new Script([offer("a"), "wait"], stop)
    const handback = { held: 0, released: 0 }
    void claimLoop(client, 2, factory, stop.signal, {
      sleep: instantSleep([]),
      handback,
      run: () => new Promise<void>(() => {}),
    })
    await new Promise((resolve) => setTimeout(resolve, 5))
    expect(stop.signal.aborted).toBe(true)
    expect(handback).toEqual({ held: 1, released: 0 })
  })

  test("a run without model settings that fails during the stop is still finished", async () => {
    const stop = new AbortController()
    const client = new Script([], stop)
    let claims = 0
    client.claimRun = async () => {
      claims++
      if (claims > 3) {
        stop.abort()
      }
      return claims > 4 ? { run: null } : { ...offer("a"), model: undefined }
    }
    const signals: Array<AbortSignal | undefined> = []
    client.finishRun = async (input, signal?: AbortSignal) => {
      signals.push(signal)
      client.finishes.push(input)
      return { state: "failed", restarted: false }
    }
    await claimLoop(client, 1, factory, stop.signal, {
      sleep: instantSleep([]),
    })
    expect(client.finishes).toHaveLength(1)
    expect(signals).toEqual([undefined])
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
