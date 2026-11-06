import { describe, expect, it } from "vitest"

import type { Folder } from "@/folders/queries"

import type { DashboardSummary } from "./dashboard-columns"
import {
  accessSummary,
  dashboardCountText,
  groupPage,
  hasPeopleFolders,
  listSummary,
  yourAccessText,
} from "./folder-groups"

const permissions = { comment: true, edit: true, manage: true, create: true }

function folder(id: string, title: string, extra: Partial<Folder> = {}) {
  return {
    id,
    title,
    general: false,
    personal: false,
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

const checkout = folder("c", "Checkout", { dashboard_count: 2 })
const payments = folder("p", "Payments", { dashboard_count: 0 })
const general = folder("g", "General", { general: true, dashboard_count: 1 })
const folders = [checkout, payments, general]
const everything = { offset: 0, total: 3, filtered: false }

function summary(sections: ReturnType<typeof groupPage>) {
  return sections.map((section) => [
    section.folder.title,
    section.rows.map((r) => r.id),
    section.empty,
    section.continued,
  ])
}

function rows(prefix: string, count: number, inFolder: Folder) {
  return Array.from({ length: count }, (_, i) => row(`${prefix}${i}`, inFolder))
}

describe("groupPage", () => {
  it("keeps the folder order and each folder's rows in the order given", () => {
    const list = [row("a", checkout), row("b", checkout), row("d", general)]
    expect(summary(groupPage(folders, list, everything))).toEqual([
      ["Checkout", ["a", "b"], false, false],
      ["Payments", [], true, false],
      ["General", ["d"], false, false],
    ])
  })

  it("places empty folders before, between and after the loaded ones", () => {
    const before = folder("e1", "Before", { dashboard_count: 0 })
    const between = folder("e2", "Between", { dashboard_count: 0 })
    const after = folder("e3", "After", { dashboard_count: 0 })
    const a = folder("a", "A", { dashboard_count: 1 })
    const b = folder("b", "B", { dashboard_count: 1 })
    const list = [before, a, between, b, after]
    const sections = groupPage(list, [row("x", a), row("y", b)], {
      offset: 0,
      total: 2,
      filtered: false,
    })
    expect(sections.map((s) => [s.folder.title, s.empty])).toEqual([
      ["Before", true],
      ["A", false],
      ["Between", true],
      ["B", false],
      ["After", true],
    ])
  })

  it("leaves an empty folder to the page where its position falls", () => {
    const big = folder("a", "Big", { dashboard_count: 50 })
    const hole = folder("e", "Hole", { dashboard_count: 0 })
    const small = folder("b", "Small", { dashboard_count: 10 })
    const list = [big, hole, small]
    const first = groupPage(list, rows("a", 50, big), {
      offset: 0,
      total: 60,
      filtered: false,
    })
    expect(first.map((s) => s.folder.title)).toEqual(["Big"])
    const second = groupPage(list, rows("b", 10, small), {
      offset: 50,
      total: 60,
      filtered: false,
    })
    expect(second.map((s) => [s.folder.title, s.empty])).toEqual([
      ["Hole", true],
      ["Small", false],
    ])
  })

  it("shows trailing empty folders on the last page", () => {
    const big = folder("a", "Big", { dashboard_count: 60 })
    const tail = folder("e", "Tail", { dashboard_count: 0 })
    const list = [big, tail]
    const first = groupPage(list, rows("a", 50, big), {
      offset: 0,
      total: 60,
      filtered: false,
    })
    expect(first.map((s) => s.folder.title)).toEqual(["Big"])
    const last = groupPage(list, rows("a", 10, big), {
      offset: 50,
      total: 60,
      filtered: false,
    })
    expect(last.map((s) => [s.folder.title, s.empty])).toEqual([
      ["Big", false],
      ["Tail", true],
    ])
  })

  it("marks a folder that started on an earlier page as continued", () => {
    const big = folder("a", "Big", { dashboard_count: 60 })
    const small = folder("b", "Small", { dashboard_count: 5 })
    const second = groupPage(
      [big, small],
      [...rows("a", 10, big), ...rows("b", 5, small)],
      {
        offset: 50,
        total: 65,
        filtered: false,
      }
    )
    expect(second.map((s) => [s.folder.title, s.continued])).toEqual([
      ["Big", true],
      ["Small", false],
    ])
  })

  it("does not mark a folder that starts exactly at the page boundary", () => {
    const big = folder("a", "Big", { dashboard_count: 50 })
    const small = folder("b", "Small", { dashboard_count: 10 })
    const second = groupPage([big, small], rows("b", 10, small), {
      offset: 50,
      total: 60,
      filtered: false,
    })
    expect(summary(second).map((s) => s[3])).toEqual([false])
  })

  it("shows no empty folder and no continuation when filtered", () => {
    const list = [row("a", checkout), row("d", general)]
    expect(
      summary(groupPage(folders, list, { offset: 0, total: 2, filtered: true }))
    ).toEqual([
      ["Checkout", ["a"], false, false],
      ["General", ["d"], false, false],
    ])
    const big = folder("a", "Big", { dashboard_count: 60 })
    const later = groupPage([big], rows("a", 10, big), {
      offset: 50,
      total: 60,
      filtered: true,
    })
    expect(later[0].continued).toBe(false)
  })

  it("falls back to the filtered behaviour when folder counts disagree with the total", () => {
    const big = folder("a", "Big", { dashboard_count: 60 })
    const hole = folder("e", "Hole", { dashboard_count: 0 })
    const sections = groupPage([big, hole], rows("a", 10, big), {
      offset: 50,
      total: 61,
      filtered: false,
    })
    expect(sections.map((s) => [s.folder.title, s.empty, s.continued])).toEqual(
      [["Big", false, false]]
    )
  })

  it("gives no sections for a filter that matches nothing", () => {
    expect(
      groupPage(folders, [], { offset: 0, total: 0, filtered: true })
    ).toEqual([])
  })

  it("shows every folder empty for a new instance without a filter", () => {
    const empty = [
      folder("a", "A"),
      folder("b", "B"),
      folder("g", "General", { general: true }),
    ]
    const sections = groupPage(empty, [], {
      offset: 0,
      total: 0,
      filtered: false,
    })
    expect(sections.map((s) => [s.folder.title, s.empty])).toEqual([
      ["A", true],
      ["B", true],
      ["General", true],
    ])
  })
})

const me = { kind: "user", id: "u-me", name: "Me" } as const
const someoneElse = { kind: "user", id: "u-x", name: "Xi" } as const

function myDashboards(owner: typeof me | typeof someoneElse) {
  return folder("m", "My Dashboards", { personal: true, created_by: owner })
}

describe("hasPeopleFolders", () => {
  const mine = folder("m", "My Dashboards", { personal: true })
  const base = folder("g", "General", { general: true })

  it.each([
    ["no folders", [], false],
    ["General alone", [base], false],
    ["General and My Dashboards", [base, mine], false],
    ["a folder someone created", [base, mine, checkout], true],
    ["an empty folder someone created", [base, payments], true],
  ])("%s", (_name, list, expected) => {
    expect(hasPeopleFolders(list)).toBe(expected)
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

  it("says only you for the caller's own My Dashboards", () => {
    expect(accessSummary(myDashboards(me))).toBe("Only you")
  })
})

describe("yourAccessText", () => {
  it("says nothing for an open folder", () => {
    expect(yourAccessText(payments)).toBeNull()
    expect(yourAccessText(general)).toBeNull()
  })

  it("says nothing for a personal folder", () => {
    expect(yourAccessText(myDashboards(me))).toBeNull()
    expect(yourAccessText(myDashboards(someoneElse))).toBeNull()
  })

  it.each([
    ["view", "Your access: View"],
    ["edit", "Your access: Edit"],
  ] as const)("names the %s level of a restricted folder", (level, text) => {
    expect(
      yourAccessText(folder("c", "Checkout", { restricted: true, level }))
    ).toBe(text)
  })
})

describe("listSummary", () => {
  it("uses plural and singular forms", () => {
    expect(listSummary(7, 3, 1)).toBe("7 dashboards in 3 folders")
    expect(listSummary(1, 1, 1)).toBe("1 dashboard in 1 folder")
    expect(listSummary(0, 0, 1)).toBe("0 dashboards in 0 folders")
    expect(listSummary(2, 1, 1)).toBe("2 dashboards in 1 folder")
    expect(listSummary(50, 2, 1)).toBe("50 dashboards in 2 folders")
  })

  it("names the page and the range when there are several pages", () => {
    expect(listSummary(312, 4, 2)).toBe("Page 2 of 7, dashboards 51-100 of 312")
    expect(listSummary(312, 4, 7)).toBe(
      "Page 7 of 7, dashboards 301-312 of 312"
    )
  })
})

describe("dashboardCountText", () => {
  it("uses plural and singular forms", () => {
    expect(dashboardCountText(0)).toBe("0 dashboards")
    expect(dashboardCountText(1)).toBe("1 dashboard")
    expect(dashboardCountText(3)).toBe("3 dashboards")
  })
})
