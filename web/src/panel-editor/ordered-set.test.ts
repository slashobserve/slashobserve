import { describe, expect, it } from "vitest"

import { toggleOrdered } from "./ordered-set"

const order = ["lastNotNull", "mean", "max", "min", "sum"]

describe("toggleOrdered", () => {
  it("adds a value in the order of the options", () => {
    expect(toggleOrdered(order, ["max"], "mean", true)).toEqual(["mean", "max"])
    expect(toggleOrdered(order, ["mean", "max"], "sum", true)).toEqual([
      "mean",
      "max",
      "sum",
    ])
  })

  it("removes a value and keeps the rest in order", () => {
    expect(
      toggleOrdered(order, ["lastNotNull", "mean"], "mean", false)
    ).toEqual(["lastNotNull"])
    expect(toggleOrdered(order, ["max"], "max", false)).toEqual([])
  })

  it("does not repeat a value that is already chosen", () => {
    expect(toggleOrdered(order, ["mean"], "mean", true)).toEqual(["mean"])
  })

  it("keeps values written elsewhere after the options", () => {
    expect(toggleOrdered(order, ["median"], "mean", true)).toEqual([
      "mean",
      "median",
    ])
  })
})
