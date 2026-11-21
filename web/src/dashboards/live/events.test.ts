import { describe, expect, it } from "vitest"

import { parseEvent } from "./events"

describe("parseEvent dashboard.deleted", () => {
  it("reads the dashboard id and the author's name", () => {
    expect(
      parseEvent(
        "dashboard.deleted",
        '{"type":"dashboard.deleted","dashboard_id":"d1","author":{"kind":"user","id":"u1","name":"Ana"}}'
      )
    ).toEqual({
      type: "dashboard.deleted",
      dashboardId: "d1",
      authorName: "Ana",
    })
  })

  it("uses an empty name when the author is missing", () => {
    expect(
      parseEvent(
        "dashboard.deleted",
        '{"type":"dashboard.deleted","dashboard_id":"d1"}'
      )
    ).toEqual({ type: "dashboard.deleted", dashboardId: "d1", authorName: "" })
  })

  it("ignores an event without a dashboard id", () => {
    expect(
      parseEvent(
        "dashboard.deleted",
        '{"type":"dashboard.deleted","author":{"name":"Ana"}}'
      )
    ).toBeNull()
  })
})

describe("parseEvent dashboard.moved", () => {
  it("reads the dashboard id", () => {
    expect(
      parseEvent(
        "dashboard.moved",
        '{"type":"dashboard.moved","dashboard_id":"d1","author":{"kind":"user","id":"u1","name":"Ana"}}'
      )
    ).toEqual({ type: "dashboard.moved", dashboardId: "d1" })
  })
})

describe("parseEvent proposal.updated", () => {
  it("reads the dashboard id", () => {
    expect(
      parseEvent(
        "proposal.updated",
        JSON.stringify({
          type: "proposal.updated",
          dashboard_id: "d1",
          proposal_id: "p1",
          status: "merged",
        })
      )
    ).toEqual({ type: "proposal.updated", dashboardId: "d1" })
  })
})

describe("parseEvent role.changed", () => {
  it("reads the dashboard id", () => {
    expect(
      parseEvent("role.changed", '{"dashboard_id":"d1","role":"editor"}')
    ).toEqual({ type: "role.changed", dashboardId: "d1" })
  })
})
