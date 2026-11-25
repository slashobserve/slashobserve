import { screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import {
  actionsColumn,
  dashboardColumns,
  folderColumn,
  type DashboardRow,
  type DashboardSummary,
} from "./dashboard-columns"

type Creator = DashboardSummary["creator"]

const ana = {
  kind: "user",
  id: "7f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f",
  name: "Ana",
  email: "ana@example.com",
} as const

function actorOf(overrides: Partial<Creator["actor"]>): Creator["actor"] {
  return {
    kind: "user",
    id: ana.id,
    name: "Ana",
    origin: "web",
    client: null,
    run_id: null,
    label: "Ana",
    ...overrides,
  }
}

function summary(extra: Partial<DashboardSummary> = {}): DashboardSummary {
  return {
    id: "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01",
    title: "Checkout service health",
    description: null,
    tags: [],
    latest_version: 7,
    created_at: "2026-11-06T09:00:00Z",
    updated_at: "2026-11-06T09:00:00Z",
    updated_by: ana,
    creator: {
      author: ana,
      origin: "web",
      origin_client: null,
      via: "direct",
      actor: actorOf({}),
    },
    datasource_uids: [],
    open_comment_count: 0,
    view_count: 0,
    folder: {
      id: "5d0c7a2e-2f43-4a4e-9a55-0d0b8f3c1a77",
      title: "Shop",
      general: false,
      personal: false,
    },
    ...extra,
  } as DashboardSummary
}

function rowOf(dashboard: DashboardSummary): DashboardRow {
  return { kind: "dashboard", id: dashboard.id, dashboard }
}

const all = [...dashboardColumns, folderColumn, actionsColumn]

async function cellOf(id: string, dashboard: DashboardSummary) {
  const column = all.find((candidate) => candidate.id === id)
  if (column === undefined) throw new Error(`no column ${id}`)
  const view = renderWithProviders(
    <div data-testid="cell">{column.cell(rowOf(dashboard))}</div>,
    { route: { path: "/", url: "/" } }
  )
  const cell = await screen.findByTestId("cell")
  return { cell, unmount: view.unmount }
}

describe("column widths", () => {
  it("gives Title a minimum width and no fixed width", () => {
    const title = all.find((c) => c.id === "title")
    expect(title?.minWidth).toBeGreaterThan(0)
    expect(title?.width).toBeUndefined()
  })

  it("fixes the width of every other column", () => {
    for (const c of all.filter((c) => c.id !== "title")) {
      expect(c.width, c.id).toBeGreaterThan(0)
    }
  })

  it("hides the optional columns in the order the grid runs out of room", () => {
    const hidable = all.filter((c) => c.hideBelow !== undefined)
    expect(
      hidable
        .toSorted((a, b) => (a.hideBelow ?? 0) - (b.hideBelow ?? 0))
        .map((c) => c.id)
    ).toEqual([
      "health",
      "madeBy",
      "version",
      "views",
      "datasources",
      "openComments",
    ])
  })

  it("never asks for more width than the grid has when a column shows", () => {
    const grouped = [...dashboardColumns, actionsColumn]
    const title = grouped.find((c) => c.id === "title")
    const fixed = grouped.filter(
      (c) => c.id !== "title" && c.hideBelow === undefined
    )
    for (const column of grouped.filter((c) => c.hideBelow !== undefined)) {
      const shown = grouped.filter(
        (c) => c.hideBelow !== undefined && c.hideBelow <= column.hideBelow!
      )
      const needed = [title, ...fixed, ...shown].reduce(
        (total, c) => total + (c?.width ?? c?.minWidth ?? 0),
        0
      )
      expect(needed, column.id).toBeLessThanOrEqual(column.hideBelow!)
    }
  })
})

describe("the open in a new tab link", () => {
  it("is out of the tab order and a target of at least 24 by 24 pixels", async () => {
    await cellOf("title", summary())
    const link = await screen.findByRole("link", {
      name: "Open Checkout service health in a new tab",
    })
    expect(link).toHaveAttribute("tabindex", "-1")
    expect(link).toHaveClass("size-6")
  })

  it("shows on the grid's row hover and stays visible on touch screens", async () => {
    await cellOf("title", summary())
    const link = await screen.findByRole("link", {
      name: "Open Checkout service health in a new tab",
    })
    expect(link).toHaveClass("opacity-0", "[.ag-row-hover_&]:opacity-100")
    expect(link.className).toContain("[@media(hover:none)]:opacity-100")
  })
})

describe("dashboardColumns", () => {
  it("lists the canvas columns in order", () => {
    expect(dashboardColumns.map((c) => c.header)).toEqual([
      "Title",
      "Health",
      "Made by",
      "Datasources",
      "Open comments",
      "Version",
      "Views",
      "Updated",
    ])
  })

  it("gives every column a unique id", () => {
    const ids = dashboardColumns.map((c) => c.id)
    expect(new Set(ids).size).toBe(dashboardColumns.length)
    expect(ids).toEqual([
      "title",
      "health",
      "madeBy",
      "datasources",
      "openComments",
      "version",
      "views",
      "updated",
    ])
  })

  it("right-aligns only Updated", () => {
    expect(all.filter((c) => c.align === "right").map((c) => c.id)).toEqual([
      "updated",
    ])
  })
})

describe("dashboard list cells", () => {
  it("shows the maker and the channel on one line with the email only on hover", async () => {
    const cases: { creator: Creator; text: string; hover: string }[] = [
      {
        creator: {
          author: ana,
          origin: "web",
          origin_client: null,
          via: "direct",
          actor: actorOf({}),
        },
        text: "Ana · Web",
        hover: "Ana · ana@example.com · Web",
      },
      {
        creator: {
          author: ana,
          origin: "web",
          origin_client: null,
          via: "agent",
          actor: actorOf({ run_id: "r1", label: "Observe Agent for Ana" }),
        },
        text: "Observe Agent for Ana",
        hover: "Observe Agent for Ana · ana@example.com",
      },
      {
        creator: {
          author: ana,
          origin: "mcp",
          origin_client: "claude-code",
          via: "direct",
          actor: actorOf({
            origin: "mcp",
            client: "Claude Code",
            label: "Ana (via Claude Code)",
          }),
        },
        text: "Ana (via Claude Code)",
        hover: "Ana (via Claude Code) · ana@example.com",
      },
      {
        creator: {
          author: { kind: "service_account", id: ana.id, name: "ci-deploy" },
          origin: "api",
          origin_client: null,
          via: "direct",
          actor: actorOf({
            kind: "service_account",
            name: "ci-deploy",
            origin: "api",
            label: "ci-deploy",
          }),
        },
        text: "ci-deploy · API",
        hover: "ci-deploy · service account · API",
      },
      {
        creator: {
          author: { kind: "user", id: ana.id, name: "Ana" },
          origin: null,
          origin_client: null,
          via: "direct",
          actor: actorOf({ origin: null }),
        },
        text: "Ana",
        hover: "Ana",
      },
    ]
    for (const { creator, text, hover } of cases) {
      const { cell, unmount } = await cellOf("madeBy", summary({ creator }))
      expect(cell.textContent).toBe(text)
      expect(cell.firstElementChild).toHaveAttribute("title", hover)
      expect(cell.querySelectorAll("div")).toHaveLength(1)
      unmount()
    }
  })

  it("leaves Datasources and Open comments empty when there are none", async () => {
    const empty = summary()
    for (const id of ["datasources", "openComments"]) {
      const { cell, unmount } = await cellOf(id, empty)
      expect(cell.textContent, id).toBe("")
      unmount()
    }

    const filled = summary({
      datasource_uids: ["prom", "ch"],
      open_comment_count: 3,
    })
    const datasources = await cellOf("datasources", filled)
    expect(datasources.cell.textContent).toBe("prom, ch")
    datasources.unmount()
    expect((await cellOf("openComments", filled)).cell.textContent).toBe("3")
  })

  it("shows how many people opened the dashboard in the window", async () => {
    const { cell } = await cellOf("views", summary({ view_count: 12 }))
    expect(cell.textContent).toBe("12")
  })

  it("clamps the description to one line and keeps the full text on hover", async () => {
    const description = "Requests, latency and errors by pod"
    const { cell } = await cellOf("title", summary({ description }))
    const line = within(cell).getByText(description)
    expect(line).toHaveClass("line-clamp-1")
    expect(line).toHaveAttribute("title", description)
  })
})
