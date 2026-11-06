import { describe, expect, it } from "vitest"

import {
  volumeCaption,
  volumeLabel,
  volumeTotals,
  type LogVolumeBucket,
} from "./volume"

function bucket(counts: Partial<LogVolumeBucket>): LogVolumeBucket {
  return {
    start: "2026-11-06T10:00:00Z",
    error: 0,
    warning: 0,
    info: 0,
    other: 0,
    ...counts,
  }
}

describe("volumeTotals", () => {
  it("sums each kind over the buckets", () => {
    expect(
      volumeTotals([
        bucket({ error: 2, info: 10 }),
        bucket({ error: 1, warning: 4, other: 3 }),
      ])
    ).toEqual({ error: 3, warning: 4, info: 10, other: 3 })
  })

  it("gives zero totals for no buckets", () => {
    expect(volumeTotals([])).toEqual({
      error: 0,
      warning: 0,
      info: 0,
      other: 0,
    })
  })
})

describe("volumeLabel", () => {
  it("leaves out zero kinds and groups thousands", () => {
    expect(
      volumeLabel(12431, { error: 120, warning: 400, info: 11911, other: 0 })
    ).toBe("12,431 lines: 120 error, 400 warning, 11,911 info")
  })

  it("says 0 lines when nothing matched", () => {
    expect(volumeLabel(0, volumeTotals([]))).toBe("0 lines")
  })
})

describe("volumeCaption", () => {
  it("names a relative range the way the time range control does", () => {
    expect(volumeCaption(12431, "now-1h", "now")).toBe(
      "12,431 lines in the last 1 hour"
    )
  })

  it("says in this range for an absolute range", () => {
    expect(
      volumeCaption(5, "2026-11-06T10:00:00Z", "2026-11-06T11:00:00Z")
    ).toBe("5 lines in this range")
  })
})
