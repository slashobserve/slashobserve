import { screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders, testQueryClient } from "@/test/render"

import { ImportLauncher } from "./import-launcher"
import type { GrafanaDatasource } from "./mapping"
import {
  grafanaDatasourcesQuery,
  grafanaFoldersQuery,
  type GrafanaConnection,
} from "./queries"

const connection = { id: "c1", name: "Prod Grafana" } as GrafanaConnection

function datasource(over: Partial<GrafanaDatasource>): GrafanaDatasource {
  return {
    uid: "g1",
    name: "Metrics",
    type: "prometheus",
    kind: "prometheus",
    candidates: ["prom"],
    matched_uid: "prom",
    ...over,
  }
}

function renderLauncher(datasources: GrafanaDatasource[]) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(grafanaFoldersQuery(connection.id).queryKey, [])
  queryClient.setQueryData(
    grafanaDatasourcesQuery(connection.id).queryKey,
    datasources
  )
  return renderWithProviders(
    <ImportLauncher
      connection={connection}
      onCancel={() => {}}
      onStarted={() => {}}
    />,
    { queryClient }
  )
}

describe("ImportLauncher", () => {
  it("lists each datasource the importer can map with its type and a choice", async () => {
    renderLauncher([
      datasource({}),
      datasource({ uid: "g2", name: "Logs", type: "loki", kind: "loki" }),
      datasource({ uid: "g3", name: "Odd", type: "mysql", kind: null }),
    ])
    const grid = await screen.findByRole("grid", { name: "Datasources" })
    expect(await within(grid).findByText("Metrics")).toBeInTheDocument()
    expect(within(grid).getByText("Logs")).toBeInTheDocument()
    expect(within(grid).queryByText("Odd")).not.toBeInTheDocument()
    expect(
      within(grid)
        .getAllByRole("columnheader")
        .map((header) => header.textContent)
    ).toEqual(["In Grafana", "Type", "Use in SlashObserve"])
    expect(
      within(grid).getByRole("combobox", { name: "Datasource for Metrics" })
    ).toBeInTheDocument()
    expect(
      screen.getByText(/1 datasource has a type/, { exact: false })
    ).toBeInTheDocument()
  })

  it("says so when no datasource has a type that can be mapped", async () => {
    renderLauncher([datasource({ kind: null })])
    expect(
      await screen.findByText(
        "This Grafana has no datasource of a type SlashObserve queries."
      )
    ).toBeInTheDocument()
    expect(screen.queryByRole("grid")).not.toBeInTheDocument()
  })
})
