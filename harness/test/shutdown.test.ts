import { describe, expect, test } from "bun:test"
import { exitCodes, shutdownOutcome } from "../src/shutdown"

function workThatEnds(): { work: Promise<void>; end: () => void } {
  let end = (): void => {}
  const work = new Promise<void>((resolve) => {
    end = resolve
  })
  return { work, end }
}

describe("shutdown", () => {
  test("work that ends within the grace after the stop signal is drained", async () => {
    const stop = new AbortController()
    const { work, end } = workThatEnds()
    const outcome = shutdownOutcome(work, stop.signal, 200)
    stop.abort()
    setTimeout(end, 5)
    expect(await outcome).toBe("drained")
  })

  test("work still going when the grace ends is forced", async () => {
    const stop = new AbortController()
    const { work } = workThatEnds()
    const outcome = shutdownOutcome(work, stop.signal, 20)
    const started = Date.now()
    stop.abort()
    expect(await outcome).toBe("forced")
    expect(Date.now() - started).toBeGreaterThanOrEqual(15)
  })

  test("the grace starts at the stop signal, not before", async () => {
    const stop = new AbortController()
    const { work, end } = workThatEnds()
    const outcome = shutdownOutcome(work, stop.signal, 10)
    await new Promise((resolve) => setTimeout(resolve, 40))
    stop.abort()
    end()
    expect(await outcome).toBe("drained")
  })

  test("work that fails is not reported as a shutdown", async () => {
    const stop = new AbortController()
    const outcome = shutdownOutcome(
      Promise.reject(new Error("boom")),
      stop.signal,
      20
    )
    await expect(outcome).rejects.toThrow("boom")
  })

  test("a drained stop exits 0 and a forced one does not", () => {
    expect(exitCodes.drained).toBe(0)
    expect(exitCodes.forced).not.toBe(0)
  })
})
