import { describe, expect, it } from "vitest"

import { exportFileName } from "./export-spec"

describe("exportFileName", () => {
  it("slugs the title and appends the version", () => {
    expect(exportFileName("Checkout service health", 7)).toBe(
      "checkout-service-health-v7.json"
    )
  })

  it("falls back to dashboard when nothing is left", () => {
    expect(exportFileName("///", 1)).toBe("dashboard-v1.json")
    expect(exportFileName("", 2)).toBe("dashboard-v2.json")
  })

  it("collapses runs of other characters and trims dashes", () => {
    expect(exportFileName("  --Edge & Cache (EU)--  ", 3)).toBe(
      "edge-cache-eu-v3.json"
    )
  })

  it("cuts the slug to 60 characters", () => {
    expect(exportFileName("a".repeat(80), 4)).toBe(`${"a".repeat(60)}-v4.json`)
  })

  it("does not end the slug with a dash after cutting", () => {
    expect(exportFileName(`${"a".repeat(59)} b`, 5)).toBe(
      `${"a".repeat(59)}-v5.json`
    )
  })
})
