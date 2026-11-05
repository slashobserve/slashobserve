import { describe, expect, test } from "vitest"

import { examplePrompts } from "./examples"

function labels(
  types: string[],
  dashboards: { id: string; title: string }[] = []
) {
  return examplePrompts({ types, dashboards }).map((example) => example.label)
}

describe("examplePrompts", () => {
  test.each([
    [
      "prometheus alone",
      ["prometheus"],
      ["Service health by pod, last 6h", "p95 latency by route"],
    ],
    [
      "prometheus and loki",
      ["prometheus", "loki"],
      [
        "Service health by pod, last 6h",
        "Error logs by service, last hour",
        "p95 latency by route",
      ],
    ],
    [
      "clickhouse alone",
      ["clickhouse"],
      [
        "Error logs by service, last hour",
        "Slowest traces by service, last hour",
      ],
    ],
    [
      "prometheus and clickhouse",
      ["prometheus", "clickhouse"],
      [
        "Service health by pod, last 6h",
        "Error logs by service, last hour",
        "p95 latency by route",
      ],
    ],
    ["tempo alone", ["tempo"], ["Slowest traces by service, last hour"]],
    ["no datasources", [], []],
  ])("offers examples for %s", (_name, types, expected) => {
    expect(labels(types)).toEqual(expected)
  })

  test("starts with a revision of the first dashboard when there is one", () => {
    const [first] = examplePrompts({
      types: ["prometheus"],
      dashboards: [
        { id: "d1", title: "Checkout" },
        { id: "d2", title: "Payments" },
      ],
    })
    expect(first).toEqual({
      label: "Add a latency row to Checkout",
      prompt: "Add a latency row to Checkout",
      dashboardId: "d1",
    })
  })

  test("offers no dashboard example without prometheus", () => {
    expect(labels(["loki"], [{ id: "d1", title: "Checkout" }])).toEqual([
      "Error logs by service, last hour",
    ])
  })

  test("clips a dashboard title over 40 characters", () => {
    const title = "A".repeat(41)
    expect(labels(["prometheus"], [{ id: "d1", title }])[0]).toBe(
      `Add a latency row to ${"A".repeat(40)}...`
    )
    expect(
      labels(["prometheus"], [{ id: "d1", title: "B".repeat(40) }])[0]
    ).toBe(`Add a latency row to ${"B".repeat(40)}`)
  })

  test("keeps at most 4 examples", () => {
    expect(
      labels(["prometheus", "loki", "tempo"], [{ id: "d1", title: "Checkout" }])
    ).toEqual([
      "Add a latency row to Checkout",
      "Service health by pod, last 6h",
      "Error logs by service, last hour",
      "Slowest traces by service, last hour",
    ])
  })
})
