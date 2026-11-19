import { describe, expect, test } from "vitest"

import { examplePrompts, type ExampleDashboard } from "./examples"

const errorsQuestion = "Which services had the most errors in the last hour?"
const slowestQuestion = "Which routes are slowest right now?"

function labels(
  types: string[],
  dashboards: ExampleDashboard[] = [],
  canBuild = true
) {
  return examplePrompts({ types, dashboards, canBuild }).map(
    (example) => example.label
  )
}

describe("examplePrompts", () => {
  test.each([
    [
      "prometheus alone",
      ["prometheus"],
      [
        "Service health by pod, last 6h",
        "p95 latency by route",
        errorsQuestion,
        slowestQuestion,
      ],
    ],
    [
      "prometheus and loki",
      ["prometheus", "loki"],
      [
        "Service health by pod, last 6h",
        "Error logs by service, last hour",
        errorsQuestion,
        slowestQuestion,
      ],
    ],
    [
      "clickhouse alone",
      ["clickhouse"],
      [
        "Error logs by service, last hour",
        "Slowest traces by service, last hour",
        errorsQuestion,
      ],
    ],
    [
      "prometheus and clickhouse",
      ["prometheus", "clickhouse"],
      [
        "Service health by pod, last 6h",
        "Error logs by service, last hour",
        errorsQuestion,
        slowestQuestion,
      ],
    ],
    [
      "tempo alone",
      ["tempo"],
      ["Slowest traces by service, last hour", slowestQuestion],
    ],
    ["no datasources", [], []],
  ])("offers examples for %s", (_name, types, expected) => {
    expect(labels(types)).toEqual(expected)
  })

  test("starts with a revision of the first dashboard when there is one", () => {
    const [first] = examplePrompts({
      canBuild: true,
      types: ["prometheus"],
      dashboards: [
        { id: "d1", title: "Checkout", canEdit: true },
        { id: "d2", title: "Payments", canEdit: true },
      ],
    })
    expect(first).toEqual({
      label: "Add a latency row to Checkout",
      prompt: "Add a latency row to Checkout",
      dashboardId: "d1",
    })
  })

  test("offers no dashboard example without prometheus", () => {
    expect(
      labels(["loki"], [{ id: "d1", title: "Checkout", canEdit: true }])
    ).toEqual(["Error logs by service, last hour", errorsQuestion])
  })

  test("names only a dashboard the person can edit", () => {
    const dashboards = [
      { id: "d1", title: "Not mine", canEdit: false },
      { id: "d2", title: "Mine", canEdit: true },
    ]
    expect(labels(["prometheus"], dashboards)[0]).toBe(
      "Add a latency row to Mine"
    )
    expect(labels(["prometheus"], dashboards.slice(0, 1))[0]).toBe(
      "Service health by pod, last 6h"
    )
  })

  test("clips a dashboard title over 40 characters", () => {
    const long = (title: string) => [{ id: "d1", title, canEdit: true }]
    expect(labels(["prometheus"], long("A".repeat(41)))[0]).toBe(
      `Add a latency row to ${"A".repeat(40)}...`
    )
    expect(labels(["prometheus"], long("B".repeat(40)))[0]).toBe(
      `Add a latency row to ${"B".repeat(40)}`
    )
  })

  test("keeps at most 4 prompts and reserves 2 of them for questions", () => {
    expect(
      labels(
        ["prometheus", "loki", "tempo"],
        [{ id: "d1", title: "Checkout", canEdit: true }]
      )
    ).toEqual([
      "Add a latency row to Checkout",
      "Service health by pod, last 6h",
      errorsQuestion,
      slowestQuestion,
    ])
  })

  test("a person who cannot edit is offered only questions", () => {
    expect(labels(["prometheus", "loki", "tempo"], [], false)).toEqual([
      errorsQuestion,
      slowestQuestion,
    ])
  })
})
