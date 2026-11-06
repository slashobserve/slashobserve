import { describe, expect, it } from "vitest"

import { panelsAddedSince, type SeenVersion } from "./new-panels"

const two: SeenVersion = {
  version: 2,
  spec: { panels: [{ id: "a" }, { id: "row", panels: [{ id: "b" }] }] },
}

const three: SeenVersion = {
  version: 3,
  spec: {
    panels: [
      { id: "a" },
      { id: "row", panels: [{ id: "b" }, { id: "c" }] },
      { id: "d" },
    ],
  },
}

describe("panelsAddedSince", () => {
  it("adds nothing on the first load", () => {
    expect(panelsAddedSince(undefined, three).size).toBe(0)
  })

  it("adds nothing for the same version", () => {
    expect(panelsAddedSince(three, { ...three }).size).toBe(0)
  })

  it("adds nothing when an older version arrives", () => {
    expect(panelsAddedSince(three, two).size).toBe(0)
  })

  it("names the panels and row children a newer version added", () => {
    expect([...panelsAddedSince(two, three)].sort()).toEqual(["c", "d"])
  })
})
