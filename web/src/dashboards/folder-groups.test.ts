import { describe, expect, it } from "vitest"

import type { Folder } from "@/folders/queries"

import type { DashboardSummary } from "./dashboard-columns"
import {
  accessSummary,
  dashboardCountText,
  groupByFolder,
  listSummary,
} from "./folder-groups"

const permissions = { comment: true, edit: true, manage: true, create: true }

function folder(id: string, title: string, extra: Partial<Folder> = {}) {
  return {
    id,
    title,
    general: false,
    restricted: false,
    dashboard_count: 0,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions,
    level: "admin",
    ...extra,
  } satisfies Folder
}

function row(id: string, inFolder: Folder) {
  return {
    id,
    title: id,
    folder: {
      id: inFolder.id,
      title: inFolder.title,
      general: inFolder.general,
    },
  } as DashboardSummary
}

const checkout = folder("c", "Checkout")
const payments = folder("p", "Payments")
const general = folder("g", "General", { general: true })
const folders = [checkout, payments, general]

function summary(sections: ReturnType<typeof groupByFolder>) {
  return sections.map((section) => [
    section.folder.title,
    section.rows.map((r) => r.id),
    section.empty,
  ])
}

describe("groupByFolder", () => {
  it("keeps the folder order and each folder's rows in the order given", () => {
    const rows = [
      row("a", checkout),
      row("b", checkout),
      row("c", payments),
      row("d", general),
    ]
    expect(summary(groupByFolder(folders, rows, false, false))).toEqual([
      ["Checkout", ["a", "b"], false],
      ["Payments", ["c"], false],
      ["General", ["d"], false],
    ])
  })

  it("leaves out folders after the last loaded row while more pages exist", () => {
    const rows = [row("a", checkout)]
    expect(summary(groupByFolder(folders, rows, true, false))).toEqual([
      ["Checkout", ["a"], false],
    ])
  })

  it("keeps an empty folder before the last loaded row while more pages exist", () => {
    const rows = [row("d", general)]
    expect(summary(groupByFolder(folders, rows, true, false))).toEqual([
      ["Checkout", [], true],
      ["Payments", [], true],
      ["General", ["d"], false],
    ])
  })

  it("shows an empty folder when no filter is active", () => {
    const rows = [row("a", checkout), row("d", general)]
    expect(summary(groupByFolder(folders, rows, false, false))).toEqual([
      ["Checkout", ["a"], false],
      ["Payments", [], true],
      ["General", ["d"], false],
    ])
  })

  it("hides an empty folder when a filter is active", () => {
    const rows = [row("a", checkout), row("d", general)]
    expect(summary(groupByFolder(folders, rows, false, true))).toEqual([
      ["Checkout", ["a"], false],
      ["General", ["d"], false],
    ])
  })

  it("gives no sections for a filter that matches nothing", () => {
    expect(groupByFolder(folders, [], false, true)).toEqual([])
  })

  it("shows every folder empty for a new instance without a filter", () => {
    expect(summary(groupByFolder([general], [], false, false))).toEqual([
      ["General", [], true],
    ])
  })
})

describe("accessSummary", () => {
  it("lists the grantees of a restricted folder", () => {
    expect(
      accessSummary(
        folder("c", "Checkout", {
          restricted: true,
          grantees: ["Ishan", "sre", "Priya"],
          grantee_count: 3,
        })
      )
    ).toBe("Restricted: Ishan, sre, Priya")
  })

  it("counts the grantees past the names the server sent", () => {
    expect(
      accessSummary(
        folder("c", "Checkout", {
          restricted: true,
          grantees: ["a", "b", "c", "d", "e"],
          grantee_count: 8,
        })
      )
    ).toBe("Restricted: a, b, c, d, e, and 3 more")
  })

  it("says everyone by role for an open folder", () => {
    expect(accessSummary(payments)).toBe("Everyone, by role")
    expect(accessSummary(general)).toBe("Everyone, by role")
  })
})

describe("listSummary", () => {
  it("uses plural and singular forms", () => {
    expect(listSummary(7, 3)).toBe("7 dashboards in 3 folders")
    expect(listSummary(1, 1)).toBe("1 dashboard in 1 folder")
    expect(listSummary(0, 0)).toBe("0 dashboards in 0 folders")
    expect(listSummary(2, 1)).toBe("2 dashboards in 1 folder")
  })
})

describe("dashboardCountText", () => {
  it("uses plural and singular forms", () => {
    expect(dashboardCountText(0)).toBe("0 dashboards")
    expect(dashboardCountText(1)).toBe("1 dashboard")
    expect(dashboardCountText(3)).toBe("3 dashboards")
  })
})
