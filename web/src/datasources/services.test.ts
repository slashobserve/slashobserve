import { describe, expect, it } from "vitest"

import { decodePills } from "@/logs/pills"
import type { LogsSearch } from "@/logs/search"

import type { DatasourceStatus } from "./queries"
import {
  filterSources,
  serviceSearch,
  serviceSignal,
  splitServices,
  type ServicesState,
} from "./services"

function source(uid: string, name: string): DatasourceStatus {
  return {
    uid,
    type: "loki",
    name,
    source: "file",
    healthy: true,
    health_state: "healthy",
    checked_at: "2026-11-19T00:00:00Z",
    dashboard_count: 0,
  }
}

const ready = (...names: string[]): ServicesState => ({
  status: "ready",
  field: "service_name",
  names,
  truncated: false,
})

describe("filterSources", () => {
  const sources = [source("a", "Production"), source("b", "Staging")]
  const services = new Map<string, ServicesState>([
    ["a", ready("checkout", "shop")],
    ["b", { status: "unavailable" }],
  ])

  it("keeps every source for an empty filter", () => {
    expect(filterSources(sources, services, "  ")).toEqual(sources)
  })

  it("matches a source name in any case", () => {
    expect(filterSources(sources, services, "STAG")).toEqual([sources[1]])
  })

  it("matches a service name and leaves the sources that hold it", () => {
    expect(filterSources(sources, services, "Check")).toEqual([sources[0]])
  })

  it("matches nothing when no name contains the text", () => {
    expect(filterSources(sources, services, "payments")).toEqual([])
  })

  it("does not match a source whose services are still loading or unavailable", () => {
    expect(
      filterSources(
        sources,
        new Map<string, ServicesState>([["a", { status: "loading" }]]),
        "checkout"
      )
    ).toEqual([])
  })
})

describe("splitServices", () => {
  const names = ["a", "b", "c", "d", "e"]

  it("shows the first three and counts the rest", () => {
    expect(splitServices(names, false)).toEqual({
      shown: ["a", "b", "c"],
      hidden: 2,
    })
  })

  it("shows every name once expanded or when few", () => {
    expect(splitServices(names, true)).toEqual({ shown: names, hidden: 0 })
    expect(splitServices(["a", "b", "c"], false)).toEqual({
      shown: ["a", "b", "c"],
      hidden: 0,
    })
  })
})

describe("serviceSearch", () => {
  it("filters logs with an is pill on the field the source reports", () => {
    const { f } = serviceSearch(
      "logs",
      "service_name",
      "checkout"
    ) as LogsSearch
    expect(decodePills(f)).toMatchObject([
      { kind: "is", field: "service_name", value: "checkout", negate: false },
    ])
  })

  it("sets the service filter for traces", () => {
    expect(
      serviceSearch("traces", "resource.service.name", "checkout")
    ).toEqual({ service: "checkout" })
  })
})

describe("serviceSignal", () => {
  it("covers logs and traces only", () => {
    expect(serviceSignal("logs")).toBe("logs")
    expect(serviceSignal("traces")).toBe("traces")
    expect(serviceSignal("metrics")).toBeNull()
  })
})
