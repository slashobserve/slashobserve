import { describe, expect, it } from "vitest"

import {
  recentlyArrived,
  tailExpired,
  tailPaused,
  tailReceived,
  tailResumed,
  tailSinceText,
  tailStarted,
  tailStatusText,
  tailVolume,
} from "./tail-state"

describe("tail state", () => {
  it("counts a key received twice once", () => {
    const first = tailReceived(tailStarted([]), ["a"], 1000)
    const again = tailReceived(first, ["a", "b"], 2000)
    expect([...again.arrivedAt.keys()]).toEqual(["a", "b"])
    expect(again.arrivedAt.get("a")).toBe(1000)
    expect(again.shownKeys.size).toBe(2)
  })

  it("does not mark the lines the tail started with", () => {
    const state = tailReceived(tailStarted(["a", "b"]), ["b", "c"], 1000)
    expect(recentlyArrived(state, "a")).toBe(false)
    expect(recentlyArrived(state, "b")).toBe(false)
    expect(recentlyArrived(state, "c")).toBe(true)
  })

  it("grows the pending count only while paused and shows nothing new", () => {
    const running = tailReceived(tailStarted([]), ["a"], 1000)
    expect(running.pendingCount).toBe(0)
    const paused = tailReceived(
      tailPaused(running, 1500),
      ["b", "c", "b"],
      2000
    )
    expect(paused.pendingCount).toBe(2)
    expect(recentlyArrived(paused, "b")).toBe(false)
    expect(paused.shownKeys.has("b")).toBe(false)
    const more = tailReceived(paused, ["c", "d"], 3000)
    expect(more.pendingCount).toBe(3)
  })

  it("marks the pending lines as arrived and clears the count on resume", () => {
    const paused = tailReceived(
      tailPaused(tailStarted([]), 1500),
      ["b", "c"],
      2000
    )
    const resumed = tailResumed(paused, 5000)
    expect(resumed.paused).toBe(false)
    expect(resumed.pendingCount).toBe(0)
    expect(resumed.shownKeys.has("b")).toBe(true)
    expect(recentlyArrived(resumed, "c")).toBe(true)
    expect(resumed.arrivedAt.get("c")).toBe(5000)
  })

  it("remembers where the lines held during a pause begin and when it started", () => {
    const paused = tailReceived(
      tailReceived(tailPaused(tailStarted(["a"]), 1500), ["b", "c"], 2000),
      ["d"],
      3000
    )
    expect(paused.pausedAt).toBe(1500)
    expect(paused.since).toBeNull()
    const resumed = tailResumed(paused, 5000)
    expect(resumed.since).toEqual({ key: "b", at: 1500 })
    expect(resumed.pausedAt).toBeNull()
  })

  it("keeps the earlier divider when a pause ends with nothing held", () => {
    const first = tailResumed(
      tailReceived(tailPaused(tailStarted([]), 1500), ["b"], 2000),
      3000
    )
    const idle = tailResumed(tailPaused(first, 4000), 5000)
    expect(idle.since).toEqual({ key: "b", at: 1500 })
    const next = tailResumed(
      tailReceived(tailPaused(idle, 6000), ["z"], 6500),
      7000
    )
    expect(next.since).toEqual({ key: "z", at: 6000 })
  })

  it("drops a mark after 1.5 s and keeps a younger one", () => {
    const state = tailReceived(
      tailReceived(tailStarted([]), ["old"], 1000),
      ["young"],
      2000
    )
    const later = tailExpired(state, 2500)
    expect(recentlyArrived(later, "old")).toBe(false)
    expect(recentlyArrived(later, "young")).toBe(true)
    expect(tailExpired(later, 2600)).toBe(later)
  })

  it("forgets keys the buffer no longer holds", () => {
    const state = tailReceived(
      tailStarted(["a", "b"]),
      ["c"],
      1000,
      new Set(["b", "c"])
    )
    expect([...state.shownKeys].sort()).toEqual(["b", "c"])
  })
})

describe("tailSinceText", () => {
  it("names the time the pause began", () => {
    expect(tailSinceText(new Date(2026, 10, 7, 14, 14, 31).getTime())).toBe(
      "New since 14:14"
    )
  })
})

describe("tailVolume", () => {
  const now = 1_000_000

  it("puts a line 25 s old in the third bucket from the end", () => {
    const buckets = tailVolume([now - 25_000], now)
    expect(buckets).toHaveLength(30)
    expect(buckets.map((bucket) => bucket.count).lastIndexOf(1)).toBe(27)
    expect(buckets.reduce((sum, bucket) => sum + bucket.count, 0)).toBe(1)
  })

  it("counts a line at the current time in the last bucket and ignores others outside the window", () => {
    const buckets = tailVolume([now, now - 300_001, now + 1], now)
    expect(buckets[29].count).toBe(1)
    expect(buckets.reduce((sum, bucket) => sum + bucket.count, 0)).toBe(1)
  })

  it("lists buckets oldest first", () => {
    const buckets = tailVolume([], now)
    expect(buckets[0].start).toBe(now - 300_000)
    expect(buckets[1].start - buckets[0].start).toBe(10_000)
  })
})

describe("tailStatusText", () => {
  const startedAt = new Date(2026, 10, 7, 14, 17, 2).getTime()

  it("formats the start time and both counts", () => {
    expect(tailStatusText(startedAt, 2906, 1000, "lines")).toBe(
      "Streaming since 14:17:02, 2,906 lines received, keeping the newest 1,000"
    )
  })

  it("says so when the tail has stopped", () => {
    expect(tailStatusText(startedAt, 12, 1000, "documents", false)).toBe(
      "Stopped. Streamed since 14:17:02, 12 documents received, kept the newest 1,000"
    )
  })
})
