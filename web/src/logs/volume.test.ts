import { describe, expect, it } from "vitest"

import {
  bucketTotal,
  selectionEdges,
  selectionText,
  volumeBucketText,
  volumeCaption,
  volumeFailureText,
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

describe("bucketTotal", () => {
  it("sums every kind by default and only the given kinds otherwise", () => {
    const counts = bucket({ error: 2, warning: 3, info: 10, other: 1 })
    expect(bucketTotal(counts)).toBe(16)
    expect(bucketTotal(counts, ["error", "info"])).toBe(12)
    expect(bucketTotal(counts, [])).toBe(0)
  })
})

describe("volumeBucketText", () => {
  it("names the lines and each present kind", () => {
    expect(
      volumeBucketText(bucket({ error: 12, warning: 40, info: 300 }))
    ).toBe("352 lines: 12 errors, 40 warnings, 300 info")
  })

  it("uses the singular for one", () => {
    expect(volumeBucketText(bucket({ error: 1, other: 1 }))).toBe(
      "2 lines: 1 error, 1 other"
    )
    expect(volumeBucketText(bucket({ warning: 1 }))).toBe("1 line: 1 warning")
  })

  it("says 0 lines for an empty bucket", () => {
    expect(volumeBucketText(bucket({}))).toBe("0 lines")
  })
})

describe("selectionText", () => {
  const minutes = {
    step_ms: 60_000,
    buckets: [
      bucket({ start: "2026-11-06T14:12:00Z", info: 100 }),
      bucket({ start: "2026-11-06T14:13:00Z", info: 1000, error: 20 }),
      bucket({ start: "2026-11-06T14:14:00Z", warning: 1400 }),
      bucket({ start: "2026-11-06T14:15:00Z", other: 5 }),
    ],
  }

  it("names the edges and sums the selected buckets", () => {
    expect(selectionText(minutes, 1, 2, "UTC")).toBe(
      "Selected 14:13 to 14:15, 2,420 lines"
    )
  })

  it("ends at the last bucket's end", () => {
    expect(selectionEdges(minutes, 0, 3, "UTC")).toEqual({
      start: "14:12",
      end: "14:16",
    })
  })

  it("shows seconds for buckets shorter than a minute", () => {
    const seconds = {
      step_ms: 10_000,
      buckets: [
        bucket({ start: "2026-11-06T14:12:00Z" }),
        bucket({ start: "2026-11-06T14:12:10Z" }),
      ],
    }
    expect(selectionEdges(seconds, 0, 1, "UTC")).toEqual({
      start: "14:12:00",
      end: "14:12:20",
    })
  })

  it("shows the dates when the edges fall on different days", () => {
    const overnight = {
      step_ms: 3_600_000,
      buckets: [
        bucket({ start: "2026-11-06T23:00:00Z" }),
        bucket({ start: "2026-11-07T00:00:00Z" }),
      ],
    }
    expect(selectionEdges(overnight, 0, 1, "UTC")).toEqual({
      start: "2026-11-06 23:00",
      end: "2026-11-07 01:00",
    })
  })
})

describe("volumeFailureText", () => {
  it("puts the server's reason in the notice", () => {
    expect(volumeFailureText("The query ran out of time.")).toBe(
      "The volume query did not finish (The query ran out of time). The lines below are complete."
    )
  })
})
