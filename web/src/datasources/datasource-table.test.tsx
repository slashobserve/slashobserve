import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { datasourceKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { DatasourceTable } from "./datasource-table"
import type { DatasourceStatus } from "./queries"

function status(overrides: Partial<DatasourceStatus>): DatasourceStatus {
  return {
    uid: "metrics",
    type: "prometheus",
    name: "metrics",
    source: "app",
    healthy: true,
    checked_at: "2026-11-19T00:00:00Z",
    dashboard_count: 0,
    ...overrides,
  }
}

function renderTable(datasources: DatasourceStatus[]) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(datasourceKeys.list(), { datasources })
  return renderWithProviders(
    <DatasourceTable onAdd={() => undefined} onEdit={() => undefined} />,
    { queryClient, route: { path: "/", url: "/" } }
  )
}

describe("DatasourceTable audience", () => {
  it("marks only the Admins-only datasources, from the file or the app", async () => {
    renderTable([
      status({ uid: "open" }),
      status({ uid: "payroll", audience: "admins" }),
      status({ uid: "audit", source: "file", audience: "admins" }),
      status({ uid: "everyone", audience: "everyone" }),
    ])
    expect(await screen.findAllByText("Only Admins")).toHaveLength(2)
    const payroll = screen.getByText("payroll").closest("tr")
    expect(payroll).toHaveTextContent("Only Admins")
    expect(screen.getByText("open").closest("tr")).not.toHaveTextContent(
      "Only Admins"
    )
  })

  it("offers no Edit for an Admins-only datasource from the file", async () => {
    renderTable([status({ uid: "audit", source: "file", audience: "admins" })])
    await screen.findByText("Only Admins")
    expect(
      screen.queryByRole("button", { name: "Edit audit" })
    ).not.toBeInTheDocument()
  })
})
