import { describe, expect, it } from "vitest"

import { leavesDraft } from "./leaves-draft"

const page = "/_authed/d/$dashboardId/{-$slug}"
const editing = {
  routeId: page,
  params: { dashboardId: "d1", slug: "checkout" },
  search: { panel: "rps", edit: "query" },
}

describe("leavesDraft", () => {
  it("stays on the draft when only the slug changes", () => {
    const renamed = {
      ...editing,
      params: { dashboardId: "d1", slug: "checkout-v2" },
    }
    const bare = { ...editing, params: { dashboardId: "d1" } }
    expect(leavesDraft(editing, renamed)).toBe(false)
    expect(leavesDraft(bare, editing)).toBe(false)
  })

  it("stays on the draft when other search keys change", () => {
    const ranged = { ...editing, search: { ...editing.search, from: "now-6h" } }
    expect(leavesDraft(editing, ranged)).toBe(false)
  })

  it("leaves for another dashboard or another route", () => {
    expect(
      leavesDraft(editing, { ...editing, params: { dashboardId: "d2" } })
    ).toBe(true)
    expect(
      leavesDraft(editing, {
        ...editing,
        routeId: "/_authed/d/$dashboardId/versions",
      })
    ).toBe(true)
  })

  it("leaves when the panel, the edit key or the pinned version changes", () => {
    expect(
      leavesDraft(editing, {
        ...editing,
        search: { panel: "errors", edit: "query" },
      })
    ).toBe(true)
    expect(leavesDraft(editing, { ...editing, search: { panel: "rps" } })).toBe(
      true
    )
    expect(
      leavesDraft(editing, {
        ...editing,
        search: { ...editing.search, version: 2 },
      })
    ).toBe(true)
  })
})
