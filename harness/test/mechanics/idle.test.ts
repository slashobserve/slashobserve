import { describe, expect, test } from "bun:test"
import { IdleWatch } from "../../src/session/mechanics/idle"

const live = new AbortController().signal
const pause = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

describe("IdleWatch", () => {
  test("fires without activity and reports an idle call_timeout", async () => {
    const watch = new IdleWatch(50)
    watch.touch()
    await pause(150)
    expect(watch.fired).toBe(true)
    expect(watch.signal.aborted).toBe(true)
    const failure = watch.failure(live, live, "anthropic")
    expect(failure?.idle).toBe(true)
    expect(failure?.failure).toBe("call_timeout")
    expect(failure?.provider).toBe("anthropic")
  })

  test("resets on every touch", async () => {
    const watch = new IdleWatch(120)
    watch.touch()
    for (let i = 0; i < 8; i++) {
      await pause(30)
      watch.touch()
    }
    watch.stop()
    expect(watch.fired).toBe(false)
    expect(watch.failure(live, live, "openai")).toBeUndefined()
  })

  test("is off at zero", async () => {
    const watch = new IdleWatch(0)
    watch.touch()
    await pause(80)
    expect(watch.fired).toBe(false)
    expect(watch.signal.aborted).toBe(false)
  })

  test("stop disarms a pending timer", async () => {
    const watch = new IdleWatch(40)
    watch.touch()
    watch.stop()
    await pause(100)
    expect(watch.fired).toBe(false)
  })

  test("reports nothing when the caller or the call aborted", async () => {
    const watch = new IdleWatch(30)
    watch.touch()
    await pause(100)
    const canceled = new AbortController()
    canceled.abort()
    expect(watch.failure(canceled.signal, live, "openai")).toBeUndefined()
    expect(watch.failure(live, canceled.signal, "openai")).toBeUndefined()
  })
})
