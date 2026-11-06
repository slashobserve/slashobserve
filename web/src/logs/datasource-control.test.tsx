import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { DatasourceStatus } from "@/datasources/queries"

import { DatasourceControl } from "./datasource-control"
import type { LogsSearch } from "./search"
import { LogsWorkspaceContext, type LogsWorkspace } from "./workspace"

function status(
  uid: string,
  type: string,
  overrides: Partial<DatasourceStatus> = {}
): DatasourceStatus {
  return {
    uid,
    type,
    name: uid,
    healthy: true,
    checked_at: "2026-11-07T12:00:00Z",
    dashboard_count: 0,
    ...overrides,
  }
}

const all = [
  status("loki-demo", "loki"),
  status("prod-es", "elasticsearch", {
    index: "logs-*",
    time_field: "@timestamp",
  }),
  status("prod-os", "opensearch"),
  status("ch", "clickhouse"),
  status("broken-es", "elasticsearch", {
    healthy: false,
    message: "connection refused",
  }),
]

function renderControl(uid: string, search: LogsSearch = {}) {
  const update = vi.fn()
  const announce = vi.fn()
  const datasource = all.find((candidate) => candidate.uid === uid)!
  const workspace = {
    datasource,
    datasources: all,
    search,
    update,
    announce,
  } as unknown as LogsWorkspace
  const user = userEvent.setup()
  render(
    <LogsWorkspaceContext value={workspace}>
      <DatasourceControl />
    </LogsWorkspaceContext>
  )
  return { update, announce, user }
}

async function switchTo(
  user: ReturnType<typeof userEvent.setup>,
  name: string
) {
  await user.click(screen.getByRole("button", { name: /^(loki|prod|ch)/ }))
  const dialog = await screen.findByRole("dialog", { name: "Datasources" })
  await user.click(
    within(dialog).getByRole("radio", { name: new RegExp(name) })
  )
  await user.click(within(dialog).getByRole("button", { name: "Apply" }))
}

describe("DatasourceControl", () => {
  beforeEach(() => localStorage.clear())

  it("shows the name, type, index pattern and time field on the button", () => {
    renderControl("prod-es")
    expect(
      screen.getByRole("button", {
        name: "prod-es Elasticsearch logs-* @timestamp",
      })
    ).toHaveAttribute("aria-haspopup", "dialog")
  })

  it("shows only the name and type when the row has no index", () => {
    renderControl("loki-demo")
    expect(
      screen.getByRole("button", { name: "loki-demo Loki" })
    ).toBeInTheDocument()
  })

  it("groups the datasources by type in a fixed order", async () => {
    const { user } = renderControl("loki-demo")
    await user.click(screen.getByRole("button", { name: "loki-demo Loki" }))
    const dialog = await screen.findByRole("dialog", { name: "Datasources" })
    const headings = within(dialog)
      .getAllByRole("heading", { level: 3 })
      .map((heading) => heading.textContent)
    expect(headings).toEqual([
      "Elasticsearch",
      "OpenSearch",
      "Loki",
      "ClickHouse",
    ])
    expect(within(dialog).getAllByRole("radio")).toHaveLength(5)
    expect(
      within(dialog).getAllByText("KQL, Lucene or Query DSL")
    ).toHaveLength(3)
    expect(within(dialog).getByText("LogQL")).toBeInTheDocument()
    expect(within(dialog).getByText("SQL")).toBeInTheDocument()
  })

  it("marks an unavailable datasource but keeps it selectable", async () => {
    const { user } = renderControl("loki-demo")
    await user.click(screen.getByRole("button", { name: "loki-demo Loki" }))
    const dialog = await screen.findByRole("dialog", { name: "Datasources" })
    expect(within(dialog).getByText("unavailable")).toHaveAttribute(
      "title",
      "connection refused"
    )
    expect(
      within(dialog).getByRole("radio", { name: /broken-es/ })
    ).not.toBeDisabled()
  })

  it("filters the list by name and uid", async () => {
    const { user } = renderControl("loki-demo")
    await user.click(screen.getByRole("button", { name: "loki-demo Loki" }))
    const dialog = await screen.findByRole("dialog", { name: "Datasources" })
    await user.type(
      within(dialog).getByLabelText("Search datasources"),
      "prod-"
    )
    expect(within(dialog).getAllByRole("radio")).toHaveLength(2)
    await user.type(within(dialog).getByLabelText("Search datasources"), "zzz")
    expect(
      within(dialog).getByText("No datasources match.")
    ).toBeInTheDocument()
  })

  it("clears the query when Loki switches to ClickHouse", async () => {
    const { update, announce, user } = renderControl("loki-demo", {
      q: '{a="b"}',
      from: "now-6h",
      to: "now",
    })
    await switchTo(user, "ch")
    expect(update).toHaveBeenCalledWith({
      ds: "ch",
      q: undefined,
      ask: undefined,
      line: undefined,
      cols: undefined,
    })
    expect(announce).toHaveBeenCalledWith(
      "Switched to ch; the query was cleared"
    )
  })

  it("keeps the query when Elasticsearch switches to OpenSearch", async () => {
    const { update, announce, user } = renderControl("prod-es", {
      q: "level:error",
    })
    await switchTo(user, "prod-os")
    expect(update).toHaveBeenCalledWith({
      ds: "prod-os",
      q: "level:error",
      ask: undefined,
      line: undefined,
      cols: undefined,
    })
    expect(announce).toHaveBeenCalledWith(
      "Switched to prod-os; the query was kept"
    )
  })

  it("remembers the chosen datasource for the logs page", async () => {
    const { user } = renderControl("loki-demo")
    await switchTo(user, "ch")
    expect(
      JSON.parse(localStorage.getItem("slashobserve-explore-datasource") ?? "")
    ).toEqual({ logs: "ch" })
  })

  it("changes nothing when Cancel is pressed or the same datasource applied", async () => {
    const { update, user } = renderControl("loki-demo")
    await user.click(screen.getByRole("button", { name: "loki-demo Loki" }))
    let dialog = await screen.findByRole("dialog", { name: "Datasources" })
    await user.click(within(dialog).getByRole("button", { name: "Apply" }))
    expect(update).not.toHaveBeenCalled()
    await user.click(screen.getByRole("button", { name: "loki-demo Loki" }))
    dialog = await screen.findByRole("dialog", { name: "Datasources" })
    await user.click(within(dialog).getByRole("radio", { name: /ch/ }))
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }))
    expect(update).not.toHaveBeenCalled()
  })
})
