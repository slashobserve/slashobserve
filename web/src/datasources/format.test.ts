import { describe, expect, test } from "vitest"

import {
  exploreTargets,
  failureAdvice,
  rowLabel,
  sourceLabel,
  statusOf,
  typeLabel,
  usageLabel,
} from "./format"
import type { DatasourceStatus } from "./queries"

function makeDatasource(
  overrides: Partial<DatasourceStatus> = {}
): DatasourceStatus {
  return {
    uid: "prom",
    type: "prometheus",
    name: "Prometheus",
    source: "file",
    healthy: true,
    health_state: "healthy",
    checked_at: "2026-03-10T12:00:00Z",
    dashboard_count: 0,
    ...overrides,
  }
}

describe("typeLabel", () => {
  test("names the supported types", () => {
    expect(typeLabel("prometheus")).toBe("Prometheus")
    expect(typeLabel("clickhouse")).toBe("ClickHouse, read only")
    expect(typeLabel("loki")).toBe("Loki")
    expect(typeLabel("tempo")).toBe("Tempo")
    expect(typeLabel("jaeger")).toBe("Jaeger")
    expect(typeLabel("elasticsearch")).toBe("Elasticsearch")
    expect(typeLabel("opensearch")).toBe("OpenSearch")
  })

  test("keeps an unknown type as written", () => {
    expect(typeLabel("graphite")).toBe("graphite")
  })
})

describe("sourceLabel", () => {
  test("says in plain words where a datasource comes from", () => {
    expect(sourceLabel("file")).toBe("From file")
    expect(sourceLabel("app")).toBe("Added in the app")
    expect(sourceLabel("builtin")).toBe("Built in")
  })
})

describe("exploreTargets", () => {
  test("offers logs and traces for ClickHouse", () => {
    expect(exploreTargets("clickhouse")).toEqual(["logs", "traces"])
  })

  test("offers logs for Loki and traces for Tempo", () => {
    expect(exploreTargets("loki")).toEqual(["logs"])
    expect(exploreTargets("tempo")).toEqual(["traces"])
    expect(exploreTargets("jaeger")).toEqual(["traces"])
  })

  test("offers logs for Elasticsearch and OpenSearch", () => {
    expect(exploreTargets("elasticsearch")).toEqual(["logs"])
    expect(exploreTargets("opensearch")).toEqual(["logs"])
  })

  test("offers metrics for Prometheus", () => {
    expect(exploreTargets("prometheus")).toEqual(["metrics"])
  })

  test("offers nothing for an unknown type", () => {
    expect(exploreTargets("graphite")).toEqual([])
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

  test("a failing datasource carries the advice of its failure kind", () => {
    expect(
      statusOf(
        makeDatasource({
          healthy: false,
          health_state: "failing",
          failure_kind: "credentials",
          message: "The datasource refused the credentials",
        })
      )
    ).toEqual({
      tone: "critical",
      text: "Unavailable: The datasource refused the credentials",
      advice: "Check the username, password or token.",
    })
  })

  test("an unchecked or healthy datasource carries no advice", () => {
    expect(
      statusOf(makeDatasource({ healthy: false, health_state: "unchecked" }))
        .advice
    ).toBeUndefined()
    expect(
      statusOf(makeDatasource({ failure_kind: "transient" })).advice
    ).toBeUndefined()
  })
})

describe("failureAdvice", () => {
  test("has one fixed sentence per failure kind", () => {
    expect(failureAdvice("transient")).toBe(
      "This is usually temporary. Try again in a moment."
    )
    expect(failureAdvice("credentials")).toBe(
      "Check the username, password or token."
    )
    expect(failureAdvice("configuration")).toBe(
      "A setting needs to change before this works."
    )
    expect(failureAdvice("unsupported")).toBe(
      "Connect a version the server supports."
    )
    expect(failureAdvice("unknown")).toBe(
      "The server could not tell why the check failed."
    )
    expect(failureAdvice(undefined)).toBeUndefined()
  })
})

describe("rowLabel", () => {
  const prod = { uid: "prom-prod", name: "Prometheus" }
  const stage = { uid: "prom-stage", name: "Prometheus" }
  const logs = { uid: "loki", name: "Logs" }

  test("is the name when no other row shares it", () => {
    expect(rowLabel(logs, [prod, stage, logs])).toBe("Logs")
  })

  test("adds the id when another row has the same name", () => {
    expect(rowLabel(prod, [prod, stage, logs])).toBe("Prometheus (prom-prod)")
    expect(rowLabel(stage, [prod, stage, logs])).toBe("Prometheus (prom-stage)")
  })

  test("is the name for a lone row", () => {
    expect(rowLabel(prod, [prod])).toBe("Prometheus")
  })
})
