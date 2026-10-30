import { describe, expect, it } from "vitest"

import resolveCasesText from "../../../spec/examples/time/resolve-cases.json?raw"
import {
  describeRange,
  formatDateTime,
  formatDateTimeMs,
  formatRelative,
  formatShortRelative,
  formatTimeValue,
  isAbsoluteOrdered,
  isSubMinuteOffset,
  isTimeValue,
  parseUserTime,
  refreshToMs,
  relativeTickMs,
  requestZone,
  resolveTime,
  timeOptionsFor,
} from "./time"

type ResolveCase = {
  expr: string
  now: string
  roundUp: boolean
  options: Parameters<typeof timeOptionsFor>[0]
  want: string
}

const resolveCases = JSON.parse(resolveCasesText) as ResolveCase[]

describe("isTimeValue", () => {
  it("accepts Spec expressions, RFC 3339 and epoch milliseconds", () => {
    for (const value of [
      "now",
      "now-6h",
      "now-1d/d",
      "now/d",
      "now/fy",
      "now-1y/fQ",
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
      "2026",
      "123456789",
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
    expect(formatDateTimeMs(ms + 42)).toBe("2026-10-17 09:05:07.042")
    expect(formatDateTimeMs(ms)).toBe("2026-10-17 09:05:07.000")
    expect(formatDateTimeMs(ms + 999)).toBe("2026-10-17 09:05:07.999")
    expect(formatDateTimeMs("not a time")).toBe("not a time")
    expect(formatTimeValue("now-1h")).toBe("now-1h")
  })

  it("describes ranges", () => {
    expect(describeRange("now-6h", "now")).toBe("Last 6 hours")
  })

  it("keeps preset labels", () => {
    expect(describeRange("now/d", "now")).toBe("Today so far")
    expect(describeRange("now-1d/d", "now-1d/d")).toBe("Yesterday")
  })

  it("labels any other relative range", () => {
    expect(describeRange("now-7h", "now")).toBe("Last 7 hours")
    expect(describeRange("now-90d", "now")).toBe("Last 90 days")
  })

  it("uses the singular for one", () => {
    expect(describeRange("now-1m", "now")).toBe("Last 1 minute")
    expect(describeRange("now-1d", "now")).toBe("Last 1 day")
  })

  it("names every unit", () => {
    const cases: [string, string][] = [
      ["now-45s", "Last 45 seconds"],
      ["now-20m", "Last 20 minutes"],
      ["now-8h", "Last 8 hours"],
      ["now-3d", "Last 3 days"],
      ["now-2w", "Last 2 weeks"],
      ["now-1M", "Last 1 month"],
      ["now-6M", "Last 6 months"],
      ["now-1y", "Last 1 year"],
      ["now-2y", "Last 2 years"],
    ]
    for (const [from, label] of cases) {
      expect(describeRange(from, "now")).toBe(label)
    }
  })

  it("falls back to the raw text for everything else", () => {
    expect(describeRange("now-20m", "now-5m")).toBe("now-20m to now-5m")
    expect(describeRange("now-20m/m", "now")).toBe("now-20m/m to now")
    expect(describeRange("now-1d/d", "now")).toBe("now-1d/d to now")
    expect(describeRange("now/w", "now")).toBe("now/w to now")
    expect(describeRange("now-20m", "now/m")).toBe("now-20m to now/m")
    expect(describeRange("now+20m", "now")).toBe("now+20m to now")
    expect(describeRange("now-0m", "now")).toBe("now-0m to now")
    expect(describeRange("now-05m", "now")).toBe("now-05m to now")
    expect(describeRange("now", "now")).toBe("now to now")
    expect(describeRange("now-20x", "now")).toBe("now-20x to now")
    expect(describeRange("now-2147483647m", "now")).toBe(
      "Last 2147483647 minutes"
    )
    expect(describeRange("now-3000000000s", "now")).toBe(
      "now-3000000000s to now"
    )
    expect(describeRange("now-1000000000000000000000m", "now")).toBe(
      "now-1000000000000000000000m to now"
    )
    expect(describeRange("2026-10-17T09:00:00Z", "now")).toBe(
      "2026-10-17T09:00:00Z to now"
    )
    expect(describeRange("now-2h", "2026-10-17T09:00:00Z")).toBe(
      "now-2h to 2026-10-17T09:00:00Z"
    )
  })

  it("formats epoch ranges as local times", () => {
    expect(describeRange("1760695200000", "1760698800000")).toBe(
      `${formatDateTime(1760695200000)} to ${formatDateTime(1760698800000)}`
    )
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
  const now = Date.parse("2026-10-17T12:00:00Z")

  it("orders epoch pairs", () => {
    expect(isAbsoluteOrdered("1760000000000", "1760000001000", now)).toBe(true)
    expect(isAbsoluteOrdered("1760000001000", "1760000000000", now)).toBe(false)
    expect(isAbsoluteOrdered("1760000000000", "1760000000000", now)).toBe(false)
  })

  it("resolves relative expressions against now", () => {
    expect(isAbsoluteOrdered("now-1h", "now", now)).toBe(true)
    expect(isAbsoluteOrdered("now", "now-1h", now)).toBe(false)
    expect(isAbsoluteOrdered("now", "now", now)).toBe(false)
    expect(isAbsoluteOrdered("now+1h", "now", now)).toBe(false)
    expect(isAbsoluteOrdered("now/d", "now", now)).toBe(true)
    expect(isAbsoluteOrdered("now-1d/d", "now-1d/d", now)).toBe(true)
    expect(isAbsoluteOrdered("now-1M", "now-1y", now)).toBe(false)
    expect(isAbsoluteOrdered("now-2w/w", "now-1w/w", now)).toBe(true)
  })

  it("orders RFC 3339 values and mixed pairs", () => {
    expect(
      isAbsoluteOrdered("2026-10-17T09:00:00Z", "2026-10-17T10:00:00Z", now)
    ).toBe(true)
    expect(
      isAbsoluteOrdered("2026-10-17T10:00:00Z", "2026-10-17T09:00:00Z", now)
    ).toBe(false)
    expect(isAbsoluteOrdered("2026-10-17T13:00:00Z", "now", now)).toBe(false)
    expect(isAbsoluteOrdered("now-6h", "2026-10-17T11:00:00Z", now)).toBe(true)
  })
})

describe("refreshToMs", () => {
  it("converts and clamps", () => {
    expect(refreshToMs("1s")).toBe(5000)
    expect(refreshToMs("5s")).toBe(5000)
    expect(refreshToMs("1m")).toBe(60_000)
    expect(refreshToMs("1h")).toBe(3_600_000)
    expect(refreshToMs("24h")).toBe(86_400_000)
    expect(refreshToMs("25h")).toBe(86_400_000)
    expect(refreshToMs("600h")).toBe(86_400_000)
    expect(refreshToMs("99999h")).toBe(86_400_000)
    expect(refreshToMs("off")).toBeNull()
    expect(refreshToMs(undefined)).toBeNull()
    expect(refreshToMs("soon")).toBeNull()
  })
})

describe("formatShortRelative", () => {
  const now = Date.parse("2026-10-19T12:00:00Z")
  const second = 1000
  const minute = 60 * second
  const hour = 60 * minute
  const day = 24 * hour

  it.each([
    [-2 * second, "Just now"],
    [-12 * second, "12 s ago"],
    [-59.7 * second, "59 s ago"],
    [-52 * minute, "52 min ago"],
    [-3 * hour, "3 h ago"],
    [-30 * hour, "Yesterday"],
    [-2 * day, "2 days ago"],
    [-8 * day, "Last week"],
    [-15 * day, "2 weeks ago"],
    [-40 * day, "Last month"],
    [-70 * day, "2 months ago"],
    [-400 * day, "Last year"],
    [-800 * day, "2 years ago"],
    [30 * second, "Just now"],
    [59.7 * second, "Just now"],
    [60 * second, "In 1 min"],
    [5 * minute, "In 5 min"],
    [5 * hour, "In 5 h"],
    [30 * hour, "Tomorrow"],
    [74 * day, "In 74 days"],
    [800 * day, "In 2 years"],
  ])("formats an offset of %i ms as %s", (offset, expected) => {
    expect(formatShortRelative(new Date(now + offset).toISOString(), now)).toBe(
      expected
    )
  })

  it("returns an unparsable value unchanged", () => {
    expect(formatShortRelative("not a date", now)).toBe("not a date")
  })
})

describe("relative clock interval", () => {
  const now = Date.parse("2026-10-19T12:00:00Z")

  it("flags offsets under a minute in either direction", () => {
    const at = (offset: number) => new Date(now + offset).toISOString()
    expect(isSubMinuteOffset(at(-59_000), now)).toBe(true)
    expect(isSubMinuteOffset(at(30_000), now)).toBe(true)
    expect(isSubMinuteOffset(at(-60_000), now)).toBe(false)
    expect(isSubMinuteOffset(at(60_000), now)).toBe(false)
    expect(isSubMinuteOffset("not a date", now)).toBe(false)
  })

  it("ticks every second only while a sub-minute offset is shown", () => {
    expect(relativeTickMs(0)).toBe(30_000)
    expect(relativeTickMs(2)).toBe(1000)
  })
})

describe("resolveTime", () => {
  it("reads the shared cases", () => {
    expect(resolveCases.length).toBeGreaterThanOrEqual(40)
    for (const c of resolveCases) {
      const got = resolveTime(
        c.expr,
        Date.parse(c.now),
        c.roundUp,
        timeOptionsFor(c.options)
      )
      const label = `${c.expr} at ${c.now} roundUp=${c.roundUp} ${JSON.stringify(c.options)}`
      expect(new Date(got).toISOString(), label).toBe(
        new Date(Date.parse(c.want)).toISOString()
      )
    }
  })

  it("resolves in UTC with Monday weeks without options", () => {
    const now = Date.parse("2026-10-30T20:00:00.123Z")
    expect(new Date(resolveTime("now/w", now, false)).toISOString()).toBe(
      "2026-10-26T00:00:00.000Z"
    )
  })

  it("rejects fiscal and calendar results outside years 1 to 9999", () => {
    const now = Date.parse("2026-10-30T20:00:00Z")
    expect(resolveTime("now-10000y", now, false)).toBeNaN()
    expect(resolveTime("now+2147483647d", now, false)).toBeNaN()
  })
})

describe("timeOptionsFor", () => {
  it("maps the Spec fields", () => {
    expect(
      timeOptionsFor({
        timezone: "Asia/Kolkata",
        weekStart: "sunday",
        fiscalYearStartMonth: 3,
        nowDelay: "2h",
      })
    ).toEqual({
      zone: "Asia/Kolkata",
      weekStart: "sunday",
      fiscalYearStartMonth: 3,
      nowDelayMs: 7_200_000,
    })
  })

  it("resolves utc, an absent zone and an unknown zone in UTC", () => {
    expect(timeOptionsFor({ timezone: "utc" }).zone).toBeUndefined()
    expect(timeOptionsFor({}).zone).toBeUndefined()
    expect(timeOptionsFor({ timezone: "Mars/Base" }).zone).toBeUndefined()
  })

  it("resolves browser in the browser's zone", () => {
    expect(timeOptionsFor({ timezone: "browser" }).zone).toBe(
      Intl.DateTimeFormat().resolvedOptions().timeZone
    )
  })

  it("ignores a malformed now delay", () => {
    expect(timeOptionsFor({ nowDelay: "soon" }).nowDelayMs).toBeUndefined()
  })
})

describe("requestZone", () => {
  it("sends the browser's zone only for a browser dashboard", () => {
    expect(requestZone("browser")).toBe(
      Intl.DateTimeFormat().resolvedOptions().timeZone
    )
    for (const timezone of [undefined, "utc", "Asia/Kolkata"]) {
      expect(requestZone(timezone)).toBeUndefined()
    }
  })
})
