import { afterEach, describe, expect, it } from "vitest"

import {
  parseStoredTimeZone,
  preferredDisplayZone,
  preferredZone,
  timeZoneKey,
  usePreferences,
} from "./preferences"

afterEach(() => {
  localStorage.clear()
  usePreferences.setState({ timeZone: "browser" })
})

describe("stored preferences", () => {
  it("falls back to the defaults for anything unknown", () => {
    expect(parseStoredTimeZone(null)).toBe("browser")
    expect(parseStoredTimeZone("Mars/Base")).toBe("browser")
    expect(parseStoredTimeZone("Asia/Tokyo")).toBe("Asia/Tokyo")
    expect(parseStoredTimeZone("utc")).toBe("utc")
  })
})

describe("usePreferences", () => {
  it("stores a chosen zone and drops the default", () => {
    usePreferences.getState().setTimeZone("Asia/Tokyo")
    expect(localStorage.getItem(timeZoneKey)).toBe("Asia/Tokyo")
    usePreferences.getState().setTimeZone("browser")
    expect(localStorage.getItem(timeZoneKey)).toBeNull()
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
