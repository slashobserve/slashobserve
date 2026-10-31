import { describe, expect, it } from "vitest"

import { canCreateDashboards } from "./create-access"
import type { Folder } from "./queries"

function folder(create: boolean): Folder {
  return {
    id: "f",
    title: "Payments",
    general: false,
    restricted: false,
    dashboard_count: 0,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions: { comment: true, edit: create, manage: false, create },
    level: create ? "edit" : "comment",
  }
}

describe("canCreateDashboards", () => {
  it("is false without folders", () => {
    expect(canCreateDashboards([])).toBe(false)
  })

  it("is false when no folder allows creating", () => {
    expect(canCreateDashboards([folder(false), folder(false)])).toBe(false)
  })

  it("is true when one folder allows creating", () => {
    expect(canCreateDashboards([folder(false), folder(true)])).toBe(true)
  })
})
