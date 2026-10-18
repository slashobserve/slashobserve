import { describe, expect, it } from "vitest"

import {
  describeRange,
  formatDateTime,
  formatRelative,
  formatTimeValue,
  isAbsoluteOrdered,
  isTimeValue,
  parseUserTime,
  refreshToMs,
} from "./time"

describe("isTimeValue", () => {
  it("accepts Spec expressions, RFC 3339 and epoch milliseconds", () => {
    for (const value of [
      "now",
      "now-6h",
      "now-1d/d",
      "now/d",
      "1760695200000",
      "2026-10-17T09:00:00Z",
      "2026-10-17T09:00:00.123+05:30",
    ]) {
      expect(isTimeValue(value)).toBe(true)
    }
  })

  it("rejects everything else", () => {
    for (const value of [
      "yesterday",
      "now-0h",
      "now-6",
      "",
      "1e9",
      "now - 1h",
    ]) {
      expect(isTimeValue(value)).toBe(false)
    }
  })
})

describe("parseUserTime", () => {
  it("passes time values through trimmed", () => {
    expect(parseUserTime("  now-6h ")).toBe("now-6h")
  })

  it("parses local date-times to epoch milliseconds", () => {
    expect(parseUserTime("2026-10-17 09:00")).toBe(
      String(new Date(2026, 9, 17, 9, 0, 0).getTime())
    )
    expect(parseUserTime("2026-10-17 09:00:30")).toBe(
      String(new Date(2026, 9, 17, 9, 0, 30).getTime())
    )
  })

  it("rejects invalid input", () => {
    expect(parseUserTime("yesterday")).toBeNull()
    expect(parseUserTime("2026-02-31 09:00")).toBeNull()
    expect(parseUserTime("")).toBeNull()
  })
})

describe("formatting", () => {
  it("formats epoch values in local time and leaves others alone", () => {
    const ms = new Date(2026, 9, 17, 9, 5, 7).getTime()
    expect(formatTimeValue(String(ms))).toBe("2026-10-17 09:05:07")
    expect(formatDateTime(ms)).toBe("2026-10-17 09:05:07")
    expect(formatTimeValue("now-1h")).toBe("now-1h")
  })

  it("describes ranges", () => {
    expect(describeRange("now-6h", "now")).toBe("Last 6 hours")
    expect(describeRange("now-1d/d", "now-1d/d")).toBe("Yesterday")
    expect(describeRange("now-7h", "now")).toBe("now-7h to now")
  })

  it("formats relative times", () => {
    const now = Date.parse("2026-10-17T12:00:00Z")
    expect(formatRelative("2026-10-17T11:59:00Z", now)).toBe("1 minute ago")
    expect(formatRelative("2026-10-17T09:00:00Z", now)).toBe("3 hours ago")
    expect(formatRelative("2026-10-16T12:00:00Z", now)).toBe("yesterday")
    expect(formatRelative("2026-10-17T11:59:50Z", now)).toBe("10 seconds ago")
  })
})

describe("isAbsoluteOrdered", () => {
  it("checks only absolute pairs", () => {
    expect(isAbsoluteOrdered("100", "200")).toBe(true)
    expect(isAbsoluteOrdered("200", "100")).toBe(false)
    expect(isAbsoluteOrdered("200", "200")).toBe(false)
    expect(isAbsoluteOrdered("now-1h", "now")).toBe(true)
  })
})

describe("refreshToMs", () => {
  it("converts and clamps", () => {
    expect(refreshToMs("1s")).toBe(5000)
    expect(refreshToMs("5s")).toBe(5000)
    expect(refreshToMs("1m")).toBe(60_000)
    expect(refreshToMs("1h")).toBe(3_600_000)
    expect(refreshToMs("off")).toBeNull()
    expect(refreshToMs(undefined)).toBeNull()
    expect(refreshToMs("soon")).toBeNull()
  })
})
