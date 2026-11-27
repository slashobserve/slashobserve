import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

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
    health_state: "healthy",
    checked_at: "2026-11-19T00:00:00Z",
    dashboard_count: 0,
    ...overrides,
  }
}

function renderTable(datasources: DatasourceStatus[]) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(datasourceKeys.list(), { datasources })
  return renderWithProviders(<DatasourceTable onEdit={() => undefined} />, {
    queryClient,
    route: { path: "/", url: "/" },
  })
}

describe("DatasourceTable audience", () => {
  it("marks only the Admins-only datasources, from the file or the app", async () => {
    renderTable([
      status({ uid: "open" }),
      status({ uid: "payroll", audience: "admins" }),
      status({
        uid: "audit",
        name: "audit",
        source: "file",
        audience: "admins",
      }),
      status({ uid: "everyone", audience: "everyone" }),
    ])
    expect(await screen.findAllByText("Admins only")).toHaveLength(2)
    const payroll = screen.getByText("payroll").closest("[data-uid]")
    expect(payroll).toHaveTextContent("Admins only")
    expect(
      screen.getByText("open").closest("[data-uid]")
    ).not.toHaveTextContent("Admins only")
  })

  it("offers no Edit for an Admins-only datasource from the file", async () => {
    renderTable([
      status({
        uid: "audit",
        name: "audit",
        source: "file",
        audience: "admins",
      }),
    ])
    await screen.findByText("Admins only")
    expect(
      screen.queryByRole("button", { name: "Edit audit" })
    ).not.toBeInTheDocument()
  })
})

describe("DatasourceTable rows", () => {
  it("names the columns and tags each row with the datasource uid", async () => {
    const { container } = renderTable([status({ uid: "metrics" })])
    await screen.findByText("Recheck")
    expect(
      screen.getAllByRole("columnheader").map((header) => header.textContent)
    ).toEqual(["Name", "Type", "Status", "Used by", "Checked", "Actions"])
    expect(container.querySelector('[data-uid="metrics"]')).toHaveTextContent(
      "Prometheus"
    )
  })

  it("hands an app datasource's Edit button to the page", async () => {
    const onEdit = vi.fn()
    const queryClient = testQueryClient()
    queryClient.setQueryData(datasourceKeys.list(), {
      datasources: [status({ uid: "metrics" })],
    })
    const { user } = renderWithProviders(<DatasourceTable onEdit={onEdit} />, {
      queryClient,
      route: { path: "/", url: "/" },
    })
    await user.click(
      await screen.findByRole("button", { name: "Edit metrics" })
    )
    expect(onEdit).toHaveBeenCalledWith("metrics", expect.any(HTMLElement))
  })
})

describe("DatasourceTable row labels", () => {
  it("names a row by its name, and adds the id only when names collide", async () => {
    renderTable([
      status({ uid: "prom-a", name: "Prometheus" }),
      status({ uid: "prom-b", name: "Prometheus" }),
      status({ uid: "loki", name: "Logs", type: "loki" }),
    ])
    expect(
      await screen.findByRole("button", { name: "Edit Prometheus (prom-a)" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Recheck Prometheus (prom-b)" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Edit Logs" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Recheck Logs" })
    ).toBeInTheDocument()
  })
})

describe("DatasourceTable built-in datasource", () => {
  const builtin = status({
    uid: "slashobserve-audit",
    name: "Audit trail",
    type: "opensearch",
    source: "builtin",
    audience: "admins",
  })

  it("says it is built in and for Admins only", async () => {
    renderTable([builtin])
    const row = (await screen.findByText("Audit trail")).closest("[data-uid]")
    expect(row).toHaveTextContent("Built in")
    expect(row).toHaveTextContent("Admins only")
  })

  it("offers no Edit or Remove, only Recheck and the logs link", async () => {
    renderTable([builtin])
    await screen.findByText("Audit trail")
    expect(
      screen.queryByRole("button", { name: "Edit Audit trail" })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: /Remove Audit trail/ })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Recheck Audit trail" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("link", {
        name: "Explore logs in Audit trail",
      })
    ).toBeInTheDocument()
  })
})
