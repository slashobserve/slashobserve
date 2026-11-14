import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { datasourceKeys, exploreKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { ResultsSection } from "./results-section"
import { LogsWorkspaceContext, type LogsWorkspace } from "./workspace"

function workspace(search: LogsWorkspace["search"]): LogsWorkspace {
  return {
    datasource: {
      uid: "ds",
      type: "elasticsearch",
      name: "ds",
      healthy: true,
      checked_at: "2026-11-07T12:00:00Z",
      dashboard_count: 0,
    },
    search,
    effectiveQuery: search.q,
    range: { from: "now-1h", to: "now" },
    results: { fetchStatus: "idle", isFetching: false },
    paging: { page: 1, pageSize: 50, lostPage: null },
    tail: { shown: false, isFetching: false, stop: vi.fn() },
    mode: "query",
    update: vi.fn(),
    openContext: vi.fn(),
    labels: { open: false, setOpen: vi.fn() },
  } as unknown as LogsWorkspace
}

const noPatterns = {
  source: "server" as const,
  lines_sampled: 0,
  lines_matched: 0,
  patterns: [],
  truncated: false,
}

function renderSection(value: LogsWorkspace) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(datasourceKeys.list(), { datasources: [] })
  if (value.search.q !== undefined) {
    queryClient.setQueryData(
      exploreKeys.patterns("ds", {
        query: value.search.q,
        from: "now-1h",
        to: "now",
        limit: 20,
      }),
      noPatterns
    )
  }
  return renderWithProviders(
    <LogsWorkspaceContext value={value}>
      <ResultsSection actions={null} />
    </LogsWorkspaceContext>,
    { queryClient }
  )
}

describe("the Patterns tab", () => {
  it("is disabled until a query has run", () => {
    renderSection(workspace({}))
    expect(
      screen
        .getByRole("tab", { name: /^Patterns/ })
        .getAttribute("aria-disabled")
    ).toBe("true")
  })

  it("asks for a query when a view=patterns link has none", () => {
    renderSection(workspace({ view: "patterns" }))
    expect(screen.getByText("Write a query and press Run")).toBeTruthy()
  })

  it("stops the live tail and writes the view when opened", async () => {
    const value = workspace({ q: "level:error" })
    renderSection(value)
    await userEvent.click(screen.getByRole("tab", { name: /^Patterns/ }))
    expect(value.tail.stop).toHaveBeenCalledOnce()
    expect(value.update).toHaveBeenCalledWith({ view: "patterns" })
  })

  it("clears the view when Documents is chosen", async () => {
    const value = workspace({ q: "level:error", view: "patterns" })
    renderSection(value)
    await userEvent.click(screen.getByRole("tab", { name: "Documents" }))
    expect(value.tail.stop).not.toHaveBeenCalled()
    expect(value.update).toHaveBeenCalledWith({ view: undefined })
  })
})
