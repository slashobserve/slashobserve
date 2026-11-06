import { describe, expect, it } from "vitest"

import type { DatasourceStatus } from "@/datasources/queries"

import { optionText } from "./datasource-picker"

function datasource(
  name: string,
  type: string,
  healthy = true
): DatasourceStatus {
  return { uid: "ds", name, type, healthy } as DatasourceStatus
}

describe("optionText", () => {
  it("shows only the type when the name is the type", () => {
    expect(optionText(datasource("clickhouse", "clickhouse"))).toBe(
      "ClickHouse"
    )
    expect(optionText(datasource("ClickHouse", "clickhouse"))).toBe(
      "ClickHouse"
    )
  })

  it("shows the name and the type otherwise", () => {
    expect(optionText(datasource("checkout-logs", "clickhouse"))).toBe(
      "checkout-logs (ClickHouse)"
    )
  })

  it("marks an unhealthy datasource", () => {
    expect(optionText(datasource("checkout-logs", "loki", false))).toBe(
      "checkout-logs (Loki) - unavailable"
    )
    expect(optionText(datasource("Loki", "loki", false))).toBe(
      "Loki - unavailable"
    )
  })
})
