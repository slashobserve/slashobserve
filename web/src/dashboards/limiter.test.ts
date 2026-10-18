import { describe, expect, it } from "vitest"

import { createLimiter } from "./limiter"

function deferred<T = void>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe("createLimiter", () => {
  it("runs at most max tasks at once, in FIFO order", async () => {
    const limiter = createLimiter(2)
    const gates = [deferred(), deferred(), deferred()]
    const started: number[] = []
    const results = gates.map((gate, index) =>
      limiter.run(async () => {
        started.push(index)
        await gate.promise
        return index
      })
    )
    await Promise.resolve()
    expect(started).toEqual([0, 1])
    gates[0].resolve()
    await expect(results[0]).resolves.toBe(0)
    await Promise.resolve()
    expect(started).toEqual([0, 1, 2])
    gates[1].resolve()
    gates[2].resolve()
    await expect(Promise.all(results)).resolves.toEqual([0, 1, 2])
  })

  it("removes a task aborted while queued and rejects with the reason", async () => {
    const limiter = createLimiter(1)
    const gate = deferred()
    const first = limiter.run(() => gate.promise)
    const controller = new AbortController()
    let ran = false
    const queued = limiter.run(async () => {
      ran = true
    }, controller.signal)
    controller.abort(new Error("gone"))
    await expect(queued).rejects.toThrow("gone")
    gate.resolve()
    await first
    await Promise.resolve()
    expect(ran).toBe(false)
  })

  it("rejects immediately for an already aborted signal", async () => {
    const limiter = createLimiter(1)
    const controller = new AbortController()
    controller.abort(new Error("early"))
    await expect(limiter.run(async () => 1, controller.signal)).rejects.toThrow(
      "early"
    )
  })

  it("frees the slot when a task fails", async () => {
    const limiter = createLimiter(1)
    await expect(
      limiter.run(() => Promise.reject(new Error("boom")))
    ).rejects.toThrow("boom")
    await expect(limiter.run(async () => "ok")).resolves.toBe("ok")
  })

  it("turns a synchronous throw into a rejection", async () => {
    const limiter = createLimiter(1)
    await expect(
      limiter.run(() => {
        throw new Error("sync")
      })
    ).rejects.toThrow("sync")
    await expect(limiter.run(async () => "ok")).resolves.toBe("ok")
  })
})
