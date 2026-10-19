import { describe, expect, test } from "vitest"

import { examplePrompts } from "./examples"

describe("examplePrompts", () => {
  test("offers metric and log examples when both are healthy", () => {
    expect(examplePrompts(["prometheus", "clickhouse"])).toEqual([
      "Service health by pod, last 6h",
      "Error logs by service from ClickHouse",
      "Node CPU and memory",
      "p95 latency by route",
    ])
  })

  test("offers only metric examples for prometheus", () => {
    expect(examplePrompts(["prometheus"])).toEqual([
      "Service health by pod, last 6h",
      "Node CPU and memory",
      "p95 latency by route",
    ])
  })

  test("adds trace and log volume examples for clickhouse alone", () => {
    expect(examplePrompts(["clickhouse"])).toEqual([
      "Error logs by service from ClickHouse",
      "Slowest traces by service from ClickHouse",
      "Log volume by severity from ClickHouse",
    ])
  })

  test("offers nothing without a healthy datasource", () => {
    expect(examplePrompts([])).toEqual([])
  })
})
