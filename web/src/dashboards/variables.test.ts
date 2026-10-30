import { describe, expect, it } from "vitest"

import { ALL } from "./search"
import {
  applyVariableChange,
  normalizeMultiSelection,
  resolveSelections,
  sortSelections,
  parentKeyOf,
  variableDisplay,
  intervalOptions,
  optionTexts,
  valuesRequests,
} from "./variables"
import type { IntervalVariable, Variable } from "@/spec"

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
  datasource: "prom",
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

  it("shows the text of custom options and keeps the value selectable", () => {
    const mixed: Variable = {
      name: "tier",
      type: "custom",
      options: ["gold", { text: "Silver tier", value: "silver" }],
    }
    expect(variableDisplay(mixed, request)).toEqual({
      status: "ready",
      shown: {
        values: ["gold", "silver"],
        texts: ["gold", "Silver tier"],
        truncated: false,
      },
      selectable: ["gold", "silver"],
    })
  })

  it("leaves texts out when every custom option text equals its value", () => {
    const same: Variable = {
      name: "tier",
      type: "custom",
      options: ["gold", { text: "silver", value: "silver" }],
    }
    expect(variableDisplay(same, request).shown).toEqual({
      values: ["gold", "silver"],
      truncated: false,
    })
    expect(variableDisplay(same, request).shown).not.toHaveProperty("texts")
  })

  it("passes the texts of fetched values through", () => {
    const data = { values: ["a", "b"], texts: ["A", "B"], truncated: false }
    expect(variableDisplay(route, { ...request, data }).shown).toBe(data)
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

  it("collects resolved selections of earlier variables only", () => {
    const urlVars = { env: ["dev"], route: ["/x"], pod: ["a"] }
    const requests = valuesRequests([pod, route, env], urlVars, {})
    expect(requests[2]).toEqual({
      parents: { pod: ["a"], route: ["/x"] },
      parentsReady: true,
    })
    expect(requests[0].parents).toEqual({})
  })

  it("repairs a stale datasource value before asking for dependent values", () => {
    const ds: Variable = {
      name: "ds",
      type: "datasource",
      datasourceType: "prometheus",
      default: "prom",
    }
    const job: Variable = { ...route, name: "job" }
    const stale = { ds: ["old_uid"] }
    expect(
      valuesRequests([ds, job], stale, { ds: ["prom", "other"] })[1]
    ).toEqual({ parents: { ds: ["prom"] }, parentsReady: true })
    expect(valuesRequests([ds, job], stale, {})[1].parentsReady).toBe(false)
  })

  it("drops extra values on a single-value parent", () => {
    const single: Variable = {
      name: "env",
      type: "custom",
      options: ["a", "b"],
    }
    const job: Variable = { ...route, name: "job" }
    expect(
      valuesRequests([single, job], { env: ["a", "b"] }, {})[1].parents
    ).toEqual({ env: ["a"] })
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

describe("resolveSelections for every variable type", () => {
  const site: Variable = { name: "site", type: "constant", value: "shop" }
  const iv: IntervalVariable = {
    name: "iv",
    type: "interval",
    options: ["1m", "5m"],
    auto: true,
  }
  const plainIv: IntervalVariable = {
    name: "iv",
    type: "interval",
    options: ["1m", "5m"],
  }
  const svc: Variable = { name: "svc", type: "textbox", default: "checkout" }
  const dsv: Variable = {
    name: "dsv",
    type: "datasource",
    datasourceType: "prometheus",
  }
  const af: Variable = { name: "af", type: "adhoc", datasource: "prom" }

  function resolve(
    variable: Variable,
    urlVars: Record<string, string[]> = {},
    options: Record<string, string[] | undefined> = {}
  ) {
    return resolveSelections([variable], urlVars, options)
  }

  it("always selects a constant's own value", () => {
    expect(resolve(site, { site: ["other"] }).selections).toEqual({
      site: ["shop"],
    })
  })

  it("offers $__auto first only when the interval variable has auto", () => {
    expect(intervalOptions(iv)).toEqual(["$__auto", "1m", "5m"])
    expect(intervalOptions(plainIv)).toEqual(["1m", "5m"])
  })

  it("selects an interval from the URL, then the default, then the first option", () => {
    expect(resolve(iv, { iv: ["5m"] }).selections).toEqual({ iv: ["5m"] })
    expect(resolve(iv, { iv: ["$__auto"] }).selections).toEqual({
      iv: ["$__auto"],
    })
    expect(resolve(iv).selections).toEqual({ iv: ["$__auto"] })
    expect(resolve({ ...plainIv, default: "5m" }).selections).toEqual({
      iv: ["5m"],
    })
    expect(resolve(plainIv).selections).toEqual({ iv: ["1m"] })
  })

  it("rejects an interval value that is not an option, and $__auto without auto", () => {
    expect(resolve(plainIv, { iv: ["$__auto"] }).selections).toEqual({
      iv: ["1m"],
    })
    expect(resolve(iv, { iv: ["7m"] }).selections).toEqual({ iv: ["$__auto"] })
  })

  it("takes a textbox from the URL, including an empty value, else its default", () => {
    expect(resolve(svc, { svc: ["payments"] }).selections).toEqual({
      svc: ["payments"],
    })
    expect(resolve(svc, { svc: [""] }).selections).toEqual({ svc: [""] })
    expect(resolve(svc).selections).toEqual({ svc: ["checkout"] })
    expect(resolve({ name: "q", type: "textbox" }).selections).toEqual({
      q: [""],
    })
  })

  it("waits for the datasource list, then prefers the URL value, the default, the first", () => {
    expect(resolve(dsv)).toEqual({
      selections: {},
      pending: true,
      waiting: ["dsv"],
    })
    const list = ["prom", "prom_tight"]
    expect(resolve(dsv, {}, { dsv: list }).selections).toEqual({
      dsv: ["prom"],
    })
    expect(
      resolve(dsv, { dsv: ["prom_tight"] }, { dsv: list }).selections
    ).toEqual({ dsv: ["prom_tight"] })
    expect(
      resolve({ ...dsv, default: "prom_tight" }, {}, { dsv: list }).selections
    ).toEqual({ dsv: ["prom_tight"] })
  })

  it("falls back when a datasource value is outside the list", () => {
    expect(
      resolve(dsv, { dsv: ["gone"] }, { dsv: ["prom"] }).selections
    ).toEqual({ dsv: ["prom"] })
    expect(
      resolve({ ...dsv, default: "gone" }, {}, { dsv: ["prom"] }).selections
    ).toEqual({ dsv: ["prom"] })
  })

  it("leaves a datasource variable with an empty list to the server", () => {
    expect(resolve(dsv, {}, { dsv: [] })).toEqual({
      selections: {},
      pending: false,
      waiting: [],
    })
  })

  it("keeps valid adhoc filters, drops the rest and sends none when empty", () => {
    expect(resolve(af).selections).toEqual({})
    expect(
      resolve(af, { af: ["job|=|api", "broken", "job|~|x", "a|=~|b|c"] })
        .selections
    ).toEqual({ af: ["job|=|api", "a|=~|b|c"] })
    expect(resolve(af, { af: ["broken"] }).selections).toEqual({})
  })

  it("keeps at most 20 adhoc filters", () => {
    const many = Array.from({ length: 25 }, (_, i) => `k${i}|=|v`)
    expect(resolve(af, { af: many }).selections.af).toHaveLength(20)
  })

  it("ignores an empty URL value on query and custom variables", () => {
    expect(
      resolveSelections([env], { env: [""] }, { env: ["prod"] }).selections
    ).toEqual({
      env: ["prod"],
    })
  })

  describe("query and custom defaults, as the server resolves them", () => {
    const withDefault = (
      variable: Variable,
      value: string | string[]
    ): Variable => ({ ...variable, default: value }) as Variable

    it("selects a single custom default instead of the first option", () => {
      expect(
        resolve(withDefault(env, "dev"), {}, { env: ["prod", "dev"] })
          .selections
      ).toEqual({ env: ["dev"] })
    })

    it("selects a query default without waiting for the values", () => {
      expect(resolve(withDefault(route, "/cart"))).toEqual({
        selections: { route: ["/cart"] },
        pending: false,
        waiting: [],
      })
    })

    it("selects every value of a multi default", () => {
      expect(
        resolve(withDefault(pod, ["b", "a"]), {}, { pod: ["a", "b"] })
          .selections
      ).toEqual({ pod: ["b", "a"] })
    })

    it("selects All for an $__all default", () => {
      expect(
        resolve(withDefault(pod, ALL), {}, { pod: ["a", "b"] }).selections
      ).toEqual({ pod: [ALL] })
    })

    it("prefers the default over includeAll", () => {
      expect(resolve(withDefault(pod, "b")).selections).toEqual({
        pod: ["b"],
      })
    })

    it("prefers the URL over the default", () => {
      expect(
        resolve(withDefault(env, "dev"), { env: ["prod"] }).selections
      ).toEqual({ env: ["prod"] })
    })

    it("falls back to the default when the URL value is empty", () => {
      expect(
        resolve(withDefault(env, "dev"), { env: [""] }).selections
      ).toEqual({ env: ["dev"] })
    })
  })

  it("never waits on or loads values for the local types", () => {
    for (const variable of [site, iv, svc, af]) {
      expect(resolve(variable).pending).toBe(false)
    }
    expect(variableDisplay(svc, requestIdle).status).toBe("ready")
    expect(variableDisplay(af, requestIdle).shown).toBeUndefined()
    expect(variableDisplay(iv, requestIdle).shown?.values).toEqual([
      "$__auto",
      "1m",
      "5m",
    ])
  })

  it("loads a datasource variable's values like a query variable's", () => {
    expect(variableDisplay(dsv, requestIdle).status).toBe("loading")
  })
})

const requestIdle = {
  data: undefined,
  last: undefined,
  parentKey: parentKeyOf({}),
  isError: false,
  isFetching: true,
}

describe("optionTexts", () => {
  it("maps each value to the text at its index", () => {
    const textOf = optionTexts({
      values: ["a", "b", "c"],
      texts: ["Alpha", "Beta", "c"],
    })
    expect([textOf("a"), textOf("b"), textOf("c")]).toEqual([
      "Alpha",
      "Beta",
      "c",
    ])
  })

  it("falls back to the value when there are no texts or the value is unknown", () => {
    expect(optionTexts({ values: ["a"] })("a")).toBe("a")
    expect(optionTexts(undefined)("x")).toBe("x")
    expect(optionTexts({ values: ["a"], texts: ["Alpha"] })("zzz")).toBe("zzz")
  })

  it("keeps the first text of a repeated value and tolerates short texts", () => {
    const textOf = optionTexts({
      values: ["a", "a", "b"],
      texts: ["One", "Two"],
    })
    expect(textOf("a")).toBe("One")
    expect(textOf("b")).toBe("b")
  })
})

describe("saved ad hoc filters", () => {
  const saved: Variable = {
    name: "af",
    type: "adhoc",
    datasource: "prom",
    default: [
      { key: "job", operator: "=", value: "api" },
      { key: "env", operator: "=~", value: "p.*" },
    ],
  }
  const plain: Variable = { name: "af", type: "adhoc", datasource: "prom" }
  const savedTexts = ["job|=|api", "env|=~|p.*"]

  it("selects the saved filters when the URL has none", () => {
    expect(resolveSelections([saved], {}, {}).selections).toEqual({
      af: savedTexts,
    })
  })

  it("lets URL filters replace the saved ones", () => {
    expect(
      resolveSelections([saved], { af: ["a|=|b"] }, {}).selections
    ).toEqual({ af: ["a|=|b"] })
  })

  it("keeps an explicit empty value as an explicit empty selection", () => {
    expect(resolveSelections([saved], { af: [""] }, {}).selections).toEqual({
      af: [""],
    })
    expect(
      resolveSelections([saved], { af: ["broken"] }, {}).selections
    ).toEqual({ af: [""] })
  })

  it("writes var-<name>= when all filters are cleared", () => {
    expect(applyVariableChange([saved], { af: ["a|=|b"] }, "af", [])).toEqual({
      af: [""],
    })
    expect(applyVariableChange([saved], {}, "af", [])).toEqual({ af: [""] })
  })

  it("keeps the parameter out of the URL when there is nothing saved to override", () => {
    expect(applyVariableChange([plain], { af: ["a|=|b"] }, "af", [])).toEqual(
      {}
    )
    expect(resolveSelections([plain], { af: [""] }, {}).selections).toEqual({})
  })
})

describe("applyVariableChange with every type", () => {
  const text: Variable = { name: "svc", type: "textbox" }
  const filters: Variable = { name: "af", type: "adhoc", datasource: "prom" }
  const ds: Variable = {
    name: "dsv",
    type: "datasource",
    datasourceType: "prometheus",
  }

  it("keeps later textbox and adhoc selections when an earlier variable changes", () => {
    expect(
      applyVariableChange(
        [env, text, filters, ds],
        { env: ["dev"], svc: ["x"], af: ["a|=|b"], dsv: ["prom"] },
        "env",
        ["prod"]
      )
    ).toEqual({ env: ["prod"], svc: ["x"], af: ["a|=|b"] })
  })

  it("keeps an emptied textbox as an empty value and drops a cleared adhoc variable", () => {
    expect(applyVariableChange([text], {}, "svc", [""])).toEqual({ svc: [""] })
    expect(applyVariableChange([filters], { af: ["a|=|b"] }, "af", [])).toEqual(
      {}
    )
  })
})
