import { describe, expect, it } from "vitest"

import { versionButtonLabel, versionButtonText } from "./version-label"

describe("versionButtonText", () => {
  it("shows the live state of the latest version", () => {
    const input = { version: 7, latest: 7, pinned: false }
    expect(versionButtonText({ ...input, status: "live" })).toBe("Live · v7")
    expect(versionButtonText({ ...input, status: "connecting" })).toBe(
      "Connecting · v7"
    )
    expect(versionButtonText({ ...input, status: "reconnecting" })).toBe(
      "Reconnecting · v7"
    )
  })

  it("shows a pinned older version against the latest", () => {
    expect(
      versionButtonText({
        version: 5,
        latest: 7,
        pinned: true,
        status: "live",
      })
    ).toBe("v5 of 7")
  })

  it("treats a pinned latest version as not pinned", () => {
    expect(
      versionButtonText({
        version: 7,
        latest: 7,
        pinned: true,
        status: "live",
      })
    ).toBe("Live · v7")
  })
})

describe("versionButtonLabel", () => {
  it("names the status for the latest version", () => {
    expect(
      versionButtonLabel({
        version: 7,
        latest: 7,
        pinned: false,
        status: "reconnecting",
      })
    ).toBe("Version 7, reconnecting. Choose a version")
  })

  it("names the pinned version and the latest", () => {
    expect(
      versionButtonLabel({
        version: 5,
        latest: 7,
        pinned: true,
        status: "live",
      })
    ).toBe("Version 5 of 7. Choose a version")
  })

  it("treats a pinned latest version as not pinned", () => {
    expect(
      versionButtonLabel({
        version: 7,
        latest: 7,
        pinned: true,
        status: "live",
      })
    ).toBe("Version 7, live. Choose a version")
  })
})
