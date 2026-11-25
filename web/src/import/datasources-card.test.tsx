import { render, screen, within } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { DatasourcesCard } from "./datasources-card"
import type { ImportSourceDatasource } from "./errors"

function datasource(
  over: Partial<ImportSourceDatasource>
): ImportSourceDatasource {
  return {
    key: "prometheus",
    name: "Metrics",
    type: "prometheus",
    kind: "prometheus",
    panels: 4,
    candidates: ["prom", "prom2"],
    match: "prom",
    ...over,
  } as ImportSourceDatasource
}

function renderCard(datasources: ImportSourceDatasource[]) {
  return render(
    <DatasourcesCard
      datasources={datasources}
      mapping={{}}
      onPick={vi.fn()}
      disabled={false}
      busy={false}
    />
  )
}

describe("DatasourcesCard", () => {
  it("lists each datasource of the source with its panel count and a choice", async () => {
    renderCard([
      datasource({}),
      datasource({ key: "loki", name: null, type: "loki", kind: "loki" }),
    ])
    const grid = await screen.findByRole("grid", { name: "Datasources" })
    expect(
      within(grid)
        .getAllByRole("columnheader")
        .map((header) => header.textContent)
    ).toEqual(["In Grafana", "Panels", "Use in SlashObserve"])
    expect(await within(grid).findByText("Metrics")).toBeInTheDocument()
    expect(
      within(grid).getByText("loki", { selector: "div" })
    ).toBeInTheDocument()
    expect(
      within(grid).getByRole("combobox", { name: "Datasource for prometheus" })
    ).toBeInTheDocument()
  })

  it("leaves out a datasource nothing uses and the importer cannot map", async () => {
    renderCard([
      datasource({}),
      datasource({ key: "mysql", kind: null, panels: 0, candidates: [] }),
    ])
    const grid = await screen.findByRole("grid", { name: "Datasources" })
    await within(grid).findByText("Metrics")
    expect(within(grid).queryByText("mysql")).not.toBeInTheDocument()
  })

  it("says a datasource kind with nothing configured is shown as placeholders", async () => {
    renderCard([datasource({ candidates: [], match: null })])
    expect(
      await screen.findByText(
        "No Prometheus datasource configured, shown as placeholders"
      )
    ).toBeInTheDocument()
  })
})
