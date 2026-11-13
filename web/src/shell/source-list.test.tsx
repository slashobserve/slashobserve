import { act, screen, within } from "@testing-library/react"
import { beforeEach, describe, expect, it } from "vitest"

import { datasourceKeys } from "@/api/keys"
import { useSourceMemory } from "@/datasources/datasource-memory"
import type { DatasourceStatus } from "@/datasources/queries"
import { useListedSources } from "@/datasources/use-sources"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { SourceList } from "./source-list"

function source(
  uid: string,
  name: string,
  overrides: Partial<DatasourceStatus> = {}
): DatasourceStatus {
  return {
    uid,
    type: "loki",
    name,
    healthy: true,
    checked_at: "2026-11-14T00:00:00Z",
    dashboard_count: 0,
    ...overrides,
  }
}

function ListedLogSources({ currentId }: { currentId: string | null }) {
  return (
    <SourceList
      kind="logs"
      {...useListedSources("logs")}
      currentId={currentId}
    />
  )
}

function renderList(
  datasources: DatasourceStatus[],
  currentId: string | null = null
) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(datasourceKeys.list(), { datasources })
  return renderWithProviders(<ListedLogSources currentId={currentId} />, {
    queryClient,
    route: { path: "/", url: "/" },
  })
}

function setRecentAtLoad(logs: string[]) {
  const loaded = { logs, traces: [] }
  useSourceMemory.setState({ ...loaded, atLoad: loaded })
}

beforeEach(() => {
  setRecentAtLoad([])
})

describe("SourceList", () => {
  it("lists the sources recent first, then by name", async () => {
    setRecentAtLoad(["c"])
    renderList([
      source("a", "Alpha"),
      source("b", "Beta"),
      source("c", "Gamma"),
    ])

    const list = await screen.findByRole("list", { name: "Log sources" })
    expect(
      within(list)
        .getAllByRole("link")
        .map((link) => link.textContent)
    ).toEqual(["GammaLogQL", "AlphaLogQL", "BetaLogQL"])
  })

  it("keeps the order on screen when a source is opened", async () => {
    setRecentAtLoad(["c"])
    renderList([
      source("a", "Alpha"),
      source("b", "Beta"),
      source("c", "Gamma"),
    ])
    const list = await screen.findByRole("list", { name: "Log sources" })

    act(() => useSourceMemory.getState().remember("logs", "b"))

    expect(
      within(list)
        .getAllByRole("link")
        .map((link) => link.textContent)
    ).toEqual(["GammaLogQL", "AlphaLogQL", "BetaLogQL"])
    expect(useSourceMemory.getState().logs).toEqual(["b", "c"])
  })

  it("links each source to its own page", async () => {
    renderList([source("a", "Alpha"), source("b", "Beta")])

    const list = await screen.findByRole("list", { name: "Log sources" })
    expect(within(list).getByRole("link", { name: /Alpha/ })).toHaveAttribute(
      "href",
      "/logs/a"
    )
  })

  it("marks only the current source", async () => {
    renderList([source("a", "Alpha"), source("b", "Beta")], "b")

    const list = await screen.findByRole("list", { name: "Log sources" })
    expect(within(list).getByRole("link", { name: /Beta/ })).toHaveAttribute(
      "aria-current",
      "page"
    )
    expect(
      within(list).getByRole("link", { name: /Alpha/ })
    ).not.toHaveAttribute("aria-current")
  })

  it("shows five sources and a More link to the list when there are more", async () => {
    renderList(
      Array.from({ length: 7 }, (_, index) =>
        source(`s${index}`, `Source ${index}`)
      )
    )

    const list = await screen.findByRole("list", { name: "Log sources" })
    expect(within(list).getAllByRole("link")).toHaveLength(6)
    expect(
      within(list).getByRole("link", { name: "More log sources" })
    ).toHaveAttribute("href", "/logs")
  })

  it("shows no More button at exactly five sources", async () => {
    renderList(
      Array.from({ length: 5 }, (_, index) =>
        source(`s${index}`, `Source ${index}`)
      )
    )

    const list = await screen.findByRole("list", { name: "Log sources" })
    expect(
      within(list).queryByRole("link", { name: "More log sources" })
    ).toBeNull()
  })

  it("lists nothing for a single source", () => {
    renderList([source("a", "Alpha")])
    expect(screen.queryByRole("list", { name: "Log sources" })).toBeNull()
  })

  it("says when a source is unavailable", async () => {
    renderList([
      source("a", "Alpha"),
      source("b", "Beta", { healthy: false, message: "connection refused" }),
    ])

    const list = await screen.findByRole("list", { name: "Log sources" })
    expect(
      within(list).getByRole("link", { name: /Beta.*unavailable/ })
    ).toHaveAttribute("title", "Beta, LogQL, unavailable: connection refused")
    expect(
      within(list).getByRole("link", { name: "Alpha LogQL" })
    ).toBeVisible()
  })
})
