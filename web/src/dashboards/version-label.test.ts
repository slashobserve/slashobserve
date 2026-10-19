import { describe, expect, it } from "vitest"

import {
  isOlderVersion,
  versionButtonLabel,
  versionButtonText,
} from "./version-label"

describe("versionButtonText", () => {
  it("shows the live state of the latest version", () => {
    const input = { version: 7, latest: 7 }
    expect(versionButtonText({ ...input, status: "live" })).toBe("Live · v7")
    expect(versionButtonText({ ...input, status: "connecting" })).toBe(
      "Connecting · v7"
    )
    expect(versionButtonText({ ...input, status: "reconnecting" })).toBe(
      "Reconnecting · v7"
    )
  })

  it("shows an older version against the latest", () => {
    expect(versionButtonText({ version: 5, latest: 7, status: "live" })).toBe(
      "v5 of 7"
    )
  })

  it("shows an older version against the latest whatever the live status", () => {
    expect(
      versionButtonText({ version: 9, latest: 10, status: "reconnecting" })
    ).toBe("v9 of 10")
  })
})

describe("isOlderVersion", () => {
  it("is true only below the latest version", () => {
    expect(isOlderVersion({ version: 9, latest: 10, status: "live" })).toBe(
      true
    )
    expect(isOlderVersion({ version: 10, latest: 10, status: "live" })).toBe(
      false
    )
  })
})

describe("versionButtonLabel", () => {
  it("starts with the visible text for the latest version", () => {
    expect(
      versionButtonLabel({ version: 7, latest: 7, status: "reconnecting" })
    ).toBe("Reconnecting · v7. Choose a version")
  })

  it("starts with the visible text for an older version", () => {
    expect(versionButtonLabel({ version: 5, latest: 7, status: "live" })).toBe(
      "v5 of 7. Choose a version"
    )
  })
})
