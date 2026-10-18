import { describe, expect, it } from "vitest"

import type { Variable } from "@/spec"

import { ALL } from "./search"
import {
  applyVariableChange,
  earlierSelections,
  normalizeMultiSelection,
  resolveSelections,
  sortSelections,
  parentKeyOf,
  variableDisplay,
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
      waiting: [],
    })
  })

  it("waits for options that have not loaded", () => {
    const result = resolveSelections([route], {}, { route: undefined })
    expect(result.pending).toBe(true)
    expect(result.waiting).toEqual(["route"])
    expect(result.selections).toEqual({})
  })

  it("does not wait when the URL already selects a value", () => {
    const result = resolveSelections([route], { route: ["/cart"] }, {})
    expect(result).toEqual({
      selections: { route: ["/cart"] },
      pending: false,
      waiting: [],
    })
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
      waiting: [],
    })
  })
})

describe("variableDisplay", () => {
  const request = {
    data: undefined,
    last: undefined,
    parentKey: parentKeyOf({}),
    isError: false,
    isFetching: true,
  }
  const old = {
    data: { values: ["/old"], truncated: true },
    parentKey: parentKeyOf({ service: ["a"] }),
  }

  it("is ready from custom options without a request", () => {
    expect(variableDisplay(env, request)).toEqual({
      status: "ready",
      shown: { values: ["prod", "dev"], truncated: false },
      selectable: ["prod", "dev"],
    })
  })

  it("shows fresh values as ready", () => {
    const data = { values: ["/a"], truncated: false }
    expect(variableDisplay(route, { ...request, data })).toEqual({
      status: "ready",
      shown: data,
      selectable: ["/a"],
    })
  })

  it("loads with nothing to show on the first request", () => {
    expect(variableDisplay(route, request)).toEqual({
      status: "loading",
      shown: undefined,
      selectable: undefined,
    })
  })

  it("keeps the old values read-only but unselectable while a changed parent reloads", () => {
    const display = variableDisplay(route, { ...request, last: old })
    expect(display.status).toBe("refreshing")
    expect(display.shown).toBe(old.data)
    expect(display.selectable).toBeUndefined()
  })

  it("keeps the old values selectable while an unchanged parent reloads", () => {
    const display = variableDisplay(route, {
      ...request,
      last: old,
      parentKey: old.parentKey,
    })
    expect(display.status).toBe("refreshing")
    expect(display.shown).toBe(old.data)
    expect(display.selectable).toEqual(["/old"])
  })

  it("does not show old values once the request is idle without data", () => {
    expect(
      variableDisplay(route, { ...request, last: old, isFetching: false })
        .status
    ).toBe("loading")
  })

  it("reports a settled error whatever was loaded before", () => {
    expect(
      variableDisplay(route, {
        ...request,
        last: old,
        isError: true,
        isFetching: false,
      }).status
    ).toBe("error")
  })

  it("shows a retry of a failed request as refreshing", () => {
    expect(
      variableDisplay(route, { ...request, last: old, isError: true }).status
    ).toBe("refreshing")
  })

  it("keeps waiting on a default when the values request failed", () => {
    const display = variableDisplay(route, { ...request, isError: true })
    expect(
      resolveSelections([route], {}, { route: display.selectable }).pending
    ).toBe(true)
  })

  it("keeps resolving defaults from the old values while an unchanged parent reloads", () => {
    const display = variableDisplay(route, {
      ...request,
      last: old,
      parentKey: old.parentKey,
    })
    const result = resolveSelections([route], {}, { route: display.selectable })
    expect(result.pending).toBe(false)
    expect(result.selections).toEqual({ route: ["/old"] })
  })

  it("keeps resolving defaults from the old values when an unchanged parent fails to reload", () => {
    const display = variableDisplay(route, {
      ...request,
      last: old,
      parentKey: old.parentKey,
      isError: true,
      isFetching: false,
    })
    const result = resolveSelections([route], {}, { route: display.selectable })
    expect(display.status).toBe("error")
    expect(result.waiting).toEqual([])
    expect(result.selections).toEqual({ route: ["/old"] })
  })

  it("waits when a changed parent fails to reload", () => {
    const display = variableDisplay(route, {
      ...request,
      last: old,
      isError: true,
      isFetching: false,
    })
    const result = resolveSelections([route], {}, { route: display.selectable })
    expect(display.status).toBe("error")
    expect(result.waiting).toEqual(["route"])
  })

  it("never resolves a default from a previous parent's options", () => {
    const display = variableDisplay(route, { ...request, last: old })
    const result = resolveSelections([route], {}, { route: display.selectable })
    expect(result.pending).toBe(true)
    expect(result.selections).toEqual({})
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
