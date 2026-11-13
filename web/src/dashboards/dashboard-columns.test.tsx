import { screen, within } from "@testing-library/react"
import { useTable } from "@tanstack/react-table"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import {
  dashboardColumns,
  listFeatures,
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
    folder: {
      id: "5d0c7a2e-2f43-4a4e-9a55-0d0b8f3c1a77",
      title: "Shop",
      general: false,
      personal: false,
    },
    ...extra,
  } as DashboardSummary
}

function Row({ dashboard }: { dashboard: DashboardSummary }) {
  const table = useTable({
    features: listFeatures,
    columns: dashboardColumns,
    data: [dashboard],
  })
  return (
    <table>
      <tbody>
        {table.getRowModel().rows.map((row) => (
          <tr key={row.id}>
            {row.getAllCells().map((cell) => (
              <td key={cell.id}>
                <table.FlexRender cell={cell} />
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

async function cellsOf(dashboard: DashboardSummary) {
  const view = renderWithProviders(<Row dashboard={dashboard} />, {
    route: { path: "/", url: "/" },
  })
  const cells = await screen.findAllByRole("cell")
  return { cells, unmount: view.unmount }
}

describe("dashboardColumns", () => {
  it("lists the canvas columns in order", () => {
    expect(dashboardColumns.map((c) => c.header)).toEqual([
      "Title",
      "Made by",
      "Datasources",
      "Open comments",
      "Version",
      "Updated",
    ])
  })

  it("gives every column a unique id", () => {
    const ids = dashboardColumns.map((c) => c.id)
    expect(new Set(ids).size).toBe(dashboardColumns.length)
    expect(ids).toEqual([
      "title",
      "madeBy",
      "datasources",
      "openComments",
      "version",
      "updated",
    ])
  })

  it("right-aligns only Updated", () => {
    const right = dashboardColumns.filter((c) =>
      c.meta?.headerClassName?.includes("text-right")
    )
    expect(right.map((c) => c.id)).toEqual(["updated"])
    expect(right[0].meta?.cellClassName).toContain("text-right")
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
      const { cells, unmount } = await cellsOf(summary({ creator }))
      expect(cells[1].textContent).toBe(text)
      expect(cells[1].firstElementChild).toHaveAttribute("title", hover)
      expect(cells[1].querySelectorAll("div")).toHaveLength(1)
      unmount()
    }
  })

  it("leaves Datasources and Open comments empty when there are none", async () => {
    const { cells, unmount } = await cellsOf(summary())
    expect(cells[2].textContent).toBe("")
    expect(cells[3].textContent).toBe("")
    unmount()

    const filled = await cellsOf(
      summary({ datasource_uids: ["prom", "ch"], open_comment_count: 3 })
    )
    expect(filled.cells[2].textContent).toBe("prom, ch")
    expect(filled.cells[3].textContent).toBe("3")
  })

  it("clamps the description to one line and keeps the full text on hover", async () => {
    const description = "Requests, latency and errors by pod"
    const { cells } = await cellsOf(summary({ description }))
    const line = within(cells[0]).getByText(description)
    expect(line).toHaveClass("line-clamp-1")
    expect(line).toHaveAttribute("title", description)
  })
})
