import { afterEach, describe, expect, it } from "vitest"

import {
  parseStoredTimeZone,
  parseStoredWeekStart,
  preferredDisplayZone,
  preferredZone,
  timeZoneKey,
  usePreferences,
  weekStartKey,
} from "./preferences"

afterEach(() => {
  localStorage.clear()
  usePreferences.setState({ timeZone: "browser", weekStart: "monday" })
})

describe("stored preferences", () => {
  it("falls back to the defaults for anything unknown", () => {
    expect(parseStoredTimeZone(null)).toBe("browser")
    expect(parseStoredTimeZone("Mars/Base")).toBe("browser")
    expect(parseStoredTimeZone("Asia/Tokyo")).toBe("Asia/Tokyo")
    expect(parseStoredTimeZone("utc")).toBe("utc")
    expect(parseStoredWeekStart(null)).toBe("monday")
    expect(parseStoredWeekStart("friday")).toBe("monday")
    expect(parseStoredWeekStart("sunday")).toBe("sunday")
  })
})

describe("usePreferences", () => {
  it("stores a chosen zone and week start and drops the default", () => {
    usePreferences.getState().setTimeZone("Asia/Tokyo")
    usePreferences.getState().setWeekStart("sunday")
    expect(localStorage.getItem(timeZoneKey)).toBe("Asia/Tokyo")
    expect(localStorage.getItem(weekStartKey)).toBe("sunday")
    usePreferences.getState().setTimeZone("browser")
    usePreferences.getState().setWeekStart("monday")
    expect(localStorage.getItem(timeZoneKey)).toBeNull()
    expect(localStorage.getItem(weekStartKey)).toBeNull()
  })

  it("ignores a zone that does not exist", () => {
    usePreferences.getState().setTimeZone("Mars/Base")
    expect(usePreferences.getState().timeZone).toBe("browser")
    expect(localStorage.getItem(timeZoneKey)).toBeNull()
  })
})

describe("preferredZone", () => {
  it("is absent for the browser's zone so nothing changes by default", () => {
    expect(preferredZone("browser")).toBeUndefined()
    expect(preferredZone("Asia/Tokyo")).toBe("Asia/Tokyo")
  })

  it("turns the choice into the zone a formatter takes", () => {
    expect(preferredDisplayZone("browser")).toBeUndefined()
    expect(preferredDisplayZone("utc")).toBe("UTC")
    expect(preferredDisplayZone("Asia/Tokyo")).toBe("Asia/Tokyo")
  })
})
