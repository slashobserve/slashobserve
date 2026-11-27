import { describe, expect, it } from "vitest"

import { pollWhileUnhealthy, type DatasourceStatus } from "./queries"

function source(uid: string, healthy: boolean): DatasourceStatus {
  return {
    uid,
    type: "loki",
    name: uid,
    source: "file",
    healthy,
    health_state: healthy ? "healthy" : "failing",
    checked_at: "2026-11-20T00:00:00Z",
    dashboard_count: 0,
  }
}

describe("pollWhileUnhealthy", () => {
  it("does not poll before the list has loaded", () => {
    expect(pollWhileUnhealthy(undefined)).toBe(false)
  })

  it("does not poll while every source is healthy", () => {
    expect(
      pollWhileUnhealthy({
        datasources: [source("a", true), source("b", true)],
      })
    ).toBe(false)
  })

  it("polls while any source is unhealthy so a source that came up is picked up without a reload", () => {
    expect(
      pollWhileUnhealthy({
        datasources: [source("a", true), source("b", false)],
      })
    ).toBe(15_000)
  })
})
