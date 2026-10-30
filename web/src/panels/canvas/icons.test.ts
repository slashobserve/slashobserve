import { describe, expect, it } from "vitest"

import { hasIcon, iconsByName, unknownIcon } from "./icons"

const namesTheImporterMeets = [
  "question-circle",
  "info-circle",
  "exclamation-triangle",
  "check-circle",
  "times-circle",
  "server",
  "database",
  "cloud",
  "bolt",
  "fire",
  "heart",
  "user",
  "users",
  "lock",
  "unlock",
  "cog",
  "wifi",
  "signal",
  "arrow-up",
  "arrow-down",
  "arrow-left",
  "arrow-right",
  "home",
  "envelope",
  "bell",
  "clock",
  "calendar",
  "chart",
  "globe",
  "link",
  "shopping-cart",
  "temperature",
  "tachometer-fast",
  "power",
  "battery-bolt",
]

describe("icon table", () => {
  it.each(namesTheImporterMeets)("maps %s to a component", (name) => {
    expect(hasIcon(name)).toBe(true)
    expect(["function", "object"]).toContain(typeof iconsByName[name])
  })

  it("covers every name the importer meets and nothing the Spec would refuse", () => {
    expect(Object.keys(iconsByName).sort()).toEqual(
      [...namesTheImporterMeets].sort()
    )
    for (const name of Object.keys(iconsByName)) {
      expect(name).toMatch(/^[a-z0-9-]{1,64}$/)
    }
  })

  it.each(["", "no-such-icon", "constructor", "__proto__", "toString"])(
    "has no icon for %j",
    (name) => {
      expect(hasIcon(name)).toBe(false)
    }
  )

  it("draws a question mark for an unknown name", () => {
    expect(unknownIcon).toBe(iconsByName["question-circle"])
  })
})
