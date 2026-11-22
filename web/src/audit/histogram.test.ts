import { describe, expect, test } from "vitest"

import {
  barHeight,
  bucketRange,
  eventCountText,
  stepText,
  tickAlign,
} from "./histogram"

describe("stepText", () => {
  test("names the largest whole unit", () => {
    expect(stepText(86_400_000)).toBe("day")
    expect(stepText(7_200_000)).toBe("2 hours")
    expect(stepText(1_800_000)).toBe("30 minutes")
    expect(stepText(90_000)).toBe("90 seconds")
    expect(stepText(250)).toBe("250 ms")
  })
})

describe("barHeight", () => {
  test("keeps an empty bucket visible and scales the rest", () => {
    expect(barHeight(0, 10)).toBe(1)
    expect(barHeight(5, 10)).toBe("50%")
    expect(barHeight(10, 10)).toBe("100%")
  })
})

describe("bucketRange", () => {
  const half = 1_800_000
  const times = [0, 1, 2, 3].map((n) => Date.UTC(2026, 10, 21, 5) + n * half)
  const window = {
    since: "2026-11-21T05:00:00.000Z",
    until: "2026-11-21T06:45:00.000Z",
  }

  test("spans one bucket for a click", () => {
    expect(bucketRange(times, half, window, 1, 1)).toEqual({
      from: "2026-11-21T05:30:00.000Z",
      to: "2026-11-21T06:00:00.000Z",
    })
  })

  test("spans every dragged bucket whichever way the drag went", () => {
    const forward = bucketRange(times, half, window, 0, 2)
    expect(forward).toEqual({
      from: "2026-11-21T05:00:00.000Z",
      to: "2026-11-21T06:30:00.000Z",
    })
    expect(bucketRange(times, half, window, 2, 0)).toEqual(forward)
  })

  test("stays inside the searched window", () => {
    expect(bucketRange(times, half, window, 3, 3)).toEqual({
      from: "2026-11-21T06:30:00.000Z",
      to: "2026-11-21T06:45:00.000Z",
    })
  })

  test("has no range for a bucket that does not exist", () => {
    expect(bucketRange(times, half, window, 9, 9)).toBe(null)
  })
})

describe("eventCountText", () => {
  test("counts one event and many", () => {
    expect(eventCountText(1)).toBe("1 event")
    expect(eventCountText(1200)).toBe("1,200 events")
  })
})

describe("tickAlign", () => {
  test("keeps the first and last labels inside the chart", () => {
    expect(tickAlign(0, 60)).toBe("start")
    expect(tickAlign(30, 60)).toBe("middle")
    expect(tickAlign(59, 60)).toBe("end")
  })
})
