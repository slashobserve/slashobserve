import { describe, expect, it } from "vitest"

import { ALL } from "@/dashboards/search"
import type { DataPanel, Variable } from "@/spec"

import {
  bakedSelections,
  blockPanelFrom,
  blockRange,
  blockVariableStates,
  blockVariables,
  variableCaption,
} from "./panel-input"

function panel(overrides: Record<string, unknown> = {}): DataPanel {
  return {
    id: "latency",
    type: "timeseries",
    title: "Latency",
    datasource: "prom",
    gridPos: { x: 4, y: 9, w: 12, h: 6 },
    queries: [{ query: "rate(http_requests_total[5m])" }],
    ...overrides,
  } as DataPanel
}

const service = {
  name: "service",
  type: "custom",
  multi: true,
  includeAll: true,
  options: [
    { text: "checkout", value: "checkout" },
    { text: "cart", value: "cart" },
  ],
} as unknown as Variable

describe("blockVariables", () => {
  const used = panel({ queries: [{ query: 'up{service=~"$service"}' }] })

  it.each([
    ["a $name reference", 'up{service="$service"}'],
    ["a ${name} reference", 'up{service="${service}"}'],
    ["a ${name:format} reference", 'up{service="${service:regex}"}'],
  ])("keeps the variable of %s", (_, query) => {
    const got = blockVariables(panel({ queries: [{ query }] }), [service], {})
    expect(got.map((variable) => variable.name)).toEqual(["service"])
  })

  it("keeps a variable beside a built-in and finds one in a later query", () => {
    const queries = [
      { query: "up" },
      { query: "rate(up{service=$service}[$__rate_interval])" },
    ]
    expect(blockVariables(panel({ queries }), [service], {})).toHaveLength(1)
  })

  it("keeps a datasource variable named by the panel", () => {
    const ds = { name: "ds", type: "datasource" } as unknown as Variable
    const got = blockVariables(
      panel({ datasource: "${ds}" }),
      [ds, service],
      {}
    )
    expect(got.map((variable) => variable.name)).toEqual(["ds"])
  })

  it.each([
    ["a plain query", "sum(rate(http_requests_total[5m]))"],
    ["a dollar sign before a digit", "up{price=~'$5'}"],
    ["a lone dollar sign", "up{x='$'}"],
    ["a built-in interval", "rate(up[$__rate_interval])"],
    ["a braced built-in", "rate(up[${__interval}])"],
    ["a built-in user name", "up{owner='${__user.name}'}"],
  ])("keeps no variable for %s", (_, query) => {
    expect(
      blockVariables(panel({ queries: [{ query }] }), [service], {})
    ).toEqual([])
  })

  it("ignores references outside the queries and the datasource", () => {
    expect(
      blockVariables(panel({ title: "$service latency" }), [service], {})
    ).toEqual([])
  })

  it("keeps the variables a kept variable's own query needs, in order", () => {
    const env = { name: "env", type: "custom" } as unknown as Variable
    const dependent = {
      name: "service",
      type: "query",
      query: 'label_values(up{env="$env"}, service)',
    } as unknown as Variable
    const unused = { name: "zone", type: "custom" } as unknown as Variable
    const got = blockVariables(used, [env, dependent, unused], {})
    expect(got.map((variable) => variable.name)).toEqual(["env", "service"])
  })

  it("records the selected value as the variable's default", () => {
    const [multi] = blockVariables(used, [service], {
      service: ["checkout", "cart"],
    })
    expect(multi).toMatchObject({ default: ["checkout", "cart"] })
    const [all] = blockVariables(used, [service], { service: [ALL] })
    expect(all).toMatchObject({ default: [ALL] })
    const single = { ...service, multi: false } as unknown as Variable
    const [one] = blockVariables(used, [single], { service: ["cart"] })
    expect(one).toMatchObject({ default: "cart" })
  })

  it("leaves the variable as defined when nothing is selected", () => {
    const [kept] = blockVariables(used, [service], {})
    expect(kept).toBeDefined()
    expect("default" in kept!).toBe(false)
  })
})

describe("bakedSelections and variableCaption", () => {
  it("reads back the values a block runs with", () => {
    const baked = blockVariables(
      panel({ queries: [{ query: "up{a=$a,b=$b}" }] }),
      [
        { ...service, name: "a" },
        { ...service, name: "b" },
      ],
      { a: ["checkout", "cart"], b: [ALL] }
    )
    const selections = bakedSelections(baked)
    expect(selections).toEqual({ a: ["checkout", "cart"], b: [ALL] })
    expect(variableCaption(selections)).toBe("with a=checkout,cart, b=All")
    expect(variableCaption({})).toBe("")
  })
})

describe("blockRange", () => {
  const now = Date.parse("2026-11-12T12:00:00Z")

  it("turns a relative range into absolute times", () => {
    expect(blockRange("now-6h", "now", now)).toEqual({
      from: "2026-11-12T06:00:00.000Z",
      to: "2026-11-12T12:00:00.000Z",
    })
  })

  it("keeps an absolute range", () => {
    expect(blockRange("1762912800000", "1762916400000", now)).toEqual({
      from: "2025-11-12T02:00:00.000Z",
      to: "2025-11-12T03:00:00.000Z",
    })
  })

  it.each([
    ["no range", undefined, undefined],
    ["only from", "now-1h", undefined],
    ["an unreadable bound", "yesterday", "now"],
    ["a range that ends before it starts", "now", "now-1h"],
  ])("gives no range for %s", (_, from, to) => {
    expect(blockRange(from, to, now)).toEqual({ from: null, to: null })
  })
})

describe("blockPanelFrom", () => {
  it("drops the repeat key and keeps the rest", () => {
    const source = panel({ repeat: "service" })
    const copy = blockPanelFrom(source)
    expect("repeat" in copy).toBe(false)
    expect(copy.queries).toEqual(source.queries)
    expect(copy.title).toBe("Latency")
  })

  it("does not change the panel it was given", () => {
    const source = panel({ repeat: "service" })
    blockPanelFrom(source)
    expect(source.repeat).toBe("service")
  })
})

describe("blockVariableStates", () => {
  const variables = [
    {
      name: "service",
      type: "custom",
      multi: true,
      options: ["checkout", "cart", "search"],
      default: ["checkout"],
    },
    {
      name: "pod",
      type: "query",
      datasource: "prom",
      query: "label_values(up, pod)",
      default: "checkout-0",
    },
    { name: "step", type: "interval", options: ["1m", "5m"], default: "5m" },
  ] as unknown as Variable[]

  it("offers a custom variable's options and a query variable's baked value", () => {
    const { states, selections } = blockVariableStates(variables, {})
    expect(states.map((state) => [state.variable.name, state.options])).toEqual(
      [
        ["service", ["checkout", "cart", "search"]],
        ["pod", ["checkout-0"]],
        ["step", ["1m", "5m"]],
      ]
    )
    expect(states.every((state) => state.status === "ready")).toBe(true)
    expect(selections).toEqual({
      service: ["checkout"],
      pod: ["checkout-0"],
      step: ["5m"],
    })
  })

  it("runs with what the person picked over the baked value", () => {
    const { states, selections } = blockVariableStates(variables, {
      service: ["cart", "search"],
      step: ["1m"],
    })
    expect(selections).toEqual({
      service: ["cart", "search"],
      pod: ["checkout-0"],
      step: ["1m"],
    })
    expect(states[0]?.selected).toEqual(["cart", "search"])
  })

  it("keeps free-entry variables at their stored value and out of the pickers", () => {
    const stored = [
      ...variables,
      { name: "svc", type: "textbox", default: "checkout" },
      { name: "filters", type: "adhoc", datasource: "prom" },
      { name: "by", type: "groupby", datasource: "prom", default: ["pod"] },
    ] as unknown as Variable[]
    const { states, fixed, selections } = blockVariableStates(stored, {
      svc: ['.+"} or up{x="'],
      filters: ["job=api"],
      by: ["node"],
    })
    expect(states.map((state) => state.variable.name)).toEqual([
      "service",
      "pod",
      "step",
    ])
    expect(fixed.map((state) => state.variable.name)).toEqual([
      "svc",
      "filters",
      "by",
    ])
    expect(selections.svc).toEqual(["checkout"])
    expect(selections.by).toEqual(["pod"])
    expect(selections.filters ?? []).toEqual([])
  })
})
