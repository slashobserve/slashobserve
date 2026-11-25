import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { throttle } from "./throttle"

beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())

describe("throttle", () => {
  it("runs on the first call at once", () => {
    const run = vi.fn()
    throttle(run, 1000).call()
    expect(run).toHaveBeenCalledTimes(1)
  })

  it("runs once more at the end of the window for calls inside it", () => {
    const run = vi.fn()
    const throttled = throttle(run, 1000)
    throttled.call()
    vi.advanceTimersByTime(200)
    throttled.call()
    throttled.call()
    expect(run).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(799)
    expect(run).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(1)
    expect(run).toHaveBeenCalledTimes(2)
  })

  it("keeps running at least once a second under calls that never stop", () => {
    const run = vi.fn()
    const throttled = throttle(run, 1000)
    for (let ms = 0; ms < 5000; ms += 100) {
      throttled.call()
      vi.advanceTimersByTime(100)
    }
    expect(run).toHaveBeenCalledTimes(6)
  })

  it("runs at once again after a quiet window", () => {
    const run = vi.fn()
    const throttled = throttle(run, 1000)
    throttled.call()
    vi.advanceTimersByTime(1500)
    throttled.call()
    expect(run).toHaveBeenCalledTimes(2)
  })

  it("drops a pending run when cancelled", () => {
    const run = vi.fn()
    const throttled = throttle(run, 1000)
    throttled.call()
    throttled.call()
    throttled.cancel()
    vi.advanceTimersByTime(2000)
    expect(run).toHaveBeenCalledTimes(1)
  })
})
