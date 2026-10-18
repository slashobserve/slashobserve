import { describe, expect, it } from "vitest"

import type { Variable } from "@/spec"

import { ALL } from "./search"
import {
  applyVariableChange,
  earlierSelections,
  normalizeMultiSelection,
  resolveSelections,
  sortSelections,
} from "./variables"

const pod: Variable = {
  name: "pod",
  type: "custom",
  options: ["a", "b"],
  multi: true,
  includeAll: true,
}
const route: Variable = {
  name: "route",
  type: "query",
  datasource: { type: "prometheus", uid: "prom" },
  query: "label_values(x, route)",
}
const env: Variable = { name: "env", type: "custom", options: ["prod", "dev"] }

describe("resolveSelections", () => {
  it("defaults to All when allowed, else to the first option", () => {
    const result = resolveSelections(
      [pod, route, env],
      {},
      {
        pod: ["a", "b"],
        route: ["/pay", "/cart"],
        env: ["prod", "dev"],
      }
    )
    expect(result).toEqual({
      selections: { pod: [ALL], route: ["/pay"], env: ["prod"] },
      pending: false,
    })
  })

  it("waits for options that have not loaded", () => {
    const result = resolveSelections([route], {}, { route: undefined })
    expect(result.pending).toBe(true)
    expect(result.selections).toEqual({})
  })

  it("does not wait when the URL already selects a value", () => {
    const result = resolveSelections([route], { route: ["/cart"] }, {})
    expect(result).toEqual({ selections: { route: ["/cart"] }, pending: false })
  })

  it("keeps one value for single variables and honors All", () => {
    expect(
      resolveSelections([env], { env: ["dev", "prod"] }, {}).selections
    ).toEqual({ env: ["dev"] })
    expect(
      resolveSelections([pod], { pod: ["a", ALL] }, {}).selections
    ).toEqual({ pod: [ALL] })
    expect(
      resolveSelections([env], { env: [ALL] }, { env: ["prod"] }).selections
    ).toEqual({ env: ["prod"] })
  })

  it("leaves a variable without options unselected", () => {
    expect(resolveSelections([route], {}, { route: [] })).toEqual({
      selections: {},
      pending: false,
    })
  })
})

describe("selection helpers", () => {
  it("sorts selection keys", () => {
    expect(Object.keys(sortSelections({ b: ["1"], a: ["2"] }))).toEqual([
      "a",
      "b",
    ])
  })

  it("collects URL selections of earlier variables only", () => {
    expect(
      earlierSelections([pod, route, env], 2, {
        env: ["dev"],
        route: ["/x"],
        pod: ["a"],
      })
    ).toEqual({ pod: ["a"], route: ["/x"] })
  })

  it("clears later variables when one changes", () => {
    expect(
      applyVariableChange(
        [pod, route, env],
        { pod: ["a"], route: ["/x"], env: ["dev"] },
        "route",
        ["/y"]
      )
    ).toEqual({ pod: ["a"], route: ["/y"] })
    expect(
      applyVariableChange(
        [pod, route],
        { pod: ["a"], route: ["/x"] },
        "pod",
        []
      )
    ).toEqual({})
  })

  it("makes All and concrete values exclusive", () => {
    expect(normalizeMultiSelection([ALL], [ALL, "a"], true)).toEqual(["a"])
    expect(normalizeMultiSelection(["a"], ["a", ALL], true)).toEqual([ALL])
    expect(normalizeMultiSelection(["a"], ["a", "b"], true)).toEqual(["a", "b"])
    expect(normalizeMultiSelection(["a"], ["a", ALL], false)).toEqual([
      "a",
      ALL,
    ])
  })
})
