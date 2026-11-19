import { describe, expect, it } from "vitest"

import { parseAnnotationTime, timeWithZone, zoneLabel } from "./time"

const now = Date.parse("2026-11-18T10:00:00Z")

describe("parseAnnotationTime", () => {
  it("reads an absolute time in the given zone", () => {
    expect(parseAnnotationTime("2026-11-18 14:22", "UTC", now)).toBe(
      Date.parse("2026-11-18T14:22:00Z")
    )
    expect(
      parseAnnotationTime("2026-11-18 14:22:30", "Asia/Kolkata", now)
    ).toBe(Date.parse("2026-11-18T08:52:30Z"))
    expect(parseAnnotationTime("2026-11-18T14:22", "UTC", now)).toBe(
      Date.parse("2026-11-18T14:22:00Z")
    )
  })

  it("reads a relative time", () => {
    expect(parseAnnotationTime("now", "UTC", now)).toBe(now)
    expect(parseAnnotationTime("now-15m", "UTC", now)).toBe(now - 15 * 60_000)
  })

  it("refuses a time that does not exist or is out of range", () => {
    expect(parseAnnotationTime("2026-02-30 10:00", "UTC", now)).toBeNull()
    expect(
      parseAnnotationTime("2026-03-08 02:30", "America/New_York", now)
    ).toBeNull()
    expect(parseAnnotationTime("1969-12-31 23:59", "UTC", now)).toBeNull()
    expect(parseAnnotationTime("2100-01-01 00:00", "UTC", now)).toBeNull()
    expect(parseAnnotationTime("9999-12-31 00:00", "UTC", now)).toBeNull()
    expect(parseAnnotationTime("yesterday", "UTC", now)).toBeNull()
    expect(parseAnnotationTime("  ", "UTC", now)).toBeNull()
  })
})

describe("timeWithZone", () => {
  it("names the zone after the time", () => {
    const ms = Date.parse("2026-11-18T21:30:00Z")
    expect(timeWithZone(ms, "UTC")).toBe("2026-11-18 21:30:00 UTC")
    expect(timeWithZone(new Date(ms).toISOString(), "Asia/Kolkata")).toBe(
      "2026-11-19 03:00:00 GMT+5:30"
    )
    expect(zoneLabel(ms, "America/New_York")).toBe("EST")
  })

  it("returns text that is not a time unchanged", () => {
    expect(timeWithZone("soon", "UTC")).toBe("soon")
  })
})
