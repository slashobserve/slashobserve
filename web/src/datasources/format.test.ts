import { describe, expect, test } from "vitest"

import { statusOf, typeLabel, usageLabel } from "./format"
import type { DatasourceStatus } from "./queries"

function makeDatasource(
  overrides: Partial<DatasourceStatus> = {}
): DatasourceStatus {
  return {
    uid: "prom",
    type: "prometheus",
    name: "Prometheus",
    healthy: true,
    checked_at: "2026-03-10T12:00:00Z",
    dashboard_count: 0,
    ...overrides,
  }
}

describe("typeLabel", () => {
  test("names the supported types", () => {
    expect(typeLabel("prometheus")).toBe("Prometheus")
    expect(typeLabel("clickhouse")).toBe("ClickHouse, read only")
  })

  test("keeps an unknown type as written", () => {
    expect(typeLabel("loki")).toBe("loki")
  })
})

describe("usageLabel", () => {
  test("says no dashboards for zero", () => {
    expect(usageLabel(0)).toBe("No dashboards")
  })

  test("uses the singular for one", () => {
    expect(usageLabel(1)).toBe("1 dashboard")
  })

  test("uses the plural for more", () => {
    expect(usageLabel(5)).toBe("5 dashboards")
  })
})

describe("statusOf", () => {
  test("a healthy datasource is ok and ignores any message", () => {
    expect(statusOf(makeDatasource({ message: "stale" }))).toEqual({
      tone: "ok",
      text: "Healthy",
    })
  })

  test("an unavailable datasource is critical and carries its message", () => {
    expect(
      statusOf(
        makeDatasource({ healthy: false, message: "connection refused" })
      )
    ).toEqual({ tone: "critical", text: "Unavailable: connection refused" })
    expect(statusOf(makeDatasource({ healthy: false }))).toEqual({
      tone: "critical",
      text: "Unavailable",
    })
  })
})
