import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { FieldsAside } from "./fields-aside"
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
    range: { from: "now-1h", to: "now" },
    results: { fetchStatus: "idle", isFetching: false },
    tail: { shown: false, isFetching: false },
    mode: "query",
    update: vi.fn(),
    openContext: vi.fn(),
    labels: { open: false, setOpen: vi.fn() },
  } as unknown as LogsWorkspace
}

describe("unfinished logs panels", () => {
  it("offers no Patterns tab and shows no plan text, even for a view=patterns link", () => {
    const { container } = renderWithProviders(
      <LogsWorkspaceContext value={workspace({ view: "patterns" })}>
        <ResultsSection />
      </LogsWorkspaceContext>
    )
    expect(screen.queryByRole("tab", { name: "Patterns" })).toBeNull()
    expect(container.textContent).not.toMatch(/plan \d+|arrives? with/i)
  })

  it("renders no Fields aside", () => {
    const { container } = renderWithProviders(<FieldsAside />)
    expect(container).toBeEmptyDOMElement()
  })
})
