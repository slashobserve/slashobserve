import { describe, expect, it } from "vitest"

import { ALL } from "../search"
import { interpolateTitle } from "./interpolate-title"

describe("interpolateTitle", () => {
  it("replaces braced and bare references", () => {
    expect(
      interpolateTitle("Pods ${pod} in $env", { pod: ["a"], env: ["x"] })
    ).toBe("Pods a in x")
  })

  it("joins several values and writes All", () => {
    expect(interpolateTitle("${pod}", { pod: ["a", "b"] })).toBe("a, b")
    expect(interpolateTitle("${pod}", { pod: [ALL] })).toBe("All")
  })

  it("keeps references it has no selection for", () => {
    expect(interpolateTitle("${pod} $other", { pod: [] })).toBe("${pod} $other")
  })

  it("leaves titles without references alone", () => {
    expect(interpolateTitle("Total", { pod: ["a"] })).toBe("Total")
  })
})
