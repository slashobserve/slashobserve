import { act, screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { observeKeys } from "@/api/keys"
import type { InputMode } from "@/ask/input-mode"
import type { DatasourceStatus } from "@/datasources/queries"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { QueryBar } from "./query-bar"
import { LogsWorkspaceContext, type LogsWorkspace } from "./workspace"

function status(uid: string, type: string): DatasourceStatus {
  return {
    uid,
    type,
    name: uid,
    healthy: true,
    checked_at: "2026-11-07T12:00:00Z",
    dashboard_count: 0,
  }
}

function setup({
  type = "elasticsearch",
  draft = "level:error",
  mode = "query",
  agentAvailable = true,
}: {
  type?: string
  draft?: string
  mode?: InputMode
  agentAvailable?: boolean
} = {}) {
  const run = vi.fn()
  const setMode = vi.fn()
  const focusQuery = vi.fn()
  const workspace = {
    datasource: status("ds", type),
    datasources: [],
    search: {},
    update: vi.fn(),
    draft,
    setDraft: vi.fn(),
    run,
    range: { from: "now-1h", to: "now" },
    mode,
    setMode,
    announce: vi.fn(),
    focusQuery,
    registerQueryFocus: vi.fn(),
    labels: { open: false, setOpen: vi.fn() },
  } as unknown as LogsWorkspace
  const queryClient = testQueryClient()
  queryClient.setQueryData(observeKeys.agent(), { available: agentAvailable })
  const view = renderWithProviders(
    <LogsWorkspaceContext value={workspace}>
      <QueryBar.ModeSwitch />
      <QueryBar />
    </LogsWorkspaceContext>,
    { queryClient }
  )
  return { ...view, run, setMode, focusQuery }
}

describe("QueryBar", () => {
  it.each([
    ["elasticsearch", "KQL"],
    ["opensearch", "KQL"],
    ["loki", "LogQL"],
    ["clickhouse", "SQL"],
  ])("names the code mode of %s %s", (type, label) => {
    setup({ type })
    const group = screen.getByRole("radiogroup", { name: "Query mode" })
    expect(group).toBeInTheDocument()
    expect(screen.getByRole("radio", { name: label })).toBeChecked()
    expect(screen.queryByRole("radio", { name: "Builder" })).toBeNull()
  })

  it("runs the query from the Run query button", async () => {
    const { run, user } = setup()
    await user.click(screen.getByRole("button", { name: "Run query" }))
    expect(run).toHaveBeenCalledOnce()
  })

  it("does not run an empty query", async () => {
    const { run, user } = setup({ draft: "  " })
    const button = screen.getByRole("button", { name: "Run query" })
    expect(button).toBeDisabled()
    await user.click(button)
    expect(run).not.toHaveBeenCalled()
  })

  it("switches to Ask on Ctrl+I and focuses the query field", async () => {
    const { setMode, focusQuery, user } = setup()
    await act(async () => {
      await user.keyboard("{Control>}i{/Control}")
    })
    expect(setMode).toHaveBeenCalledWith("ask", true)
    expect(focusQuery).toHaveBeenCalled()
  })

  it("goes back to the code mode on Ctrl+I from Ask", async () => {
    const { setMode, user } = setup({ mode: "ask" })
    await act(async () => {
      await user.keyboard("{Control>}i{/Control}")
    })
    expect(setMode).toHaveBeenCalledWith("query", true)
  })

  it("keeps Ask unavailable without a model", async () => {
    const { setMode, user } = setup({ agentAvailable: false })
    const ask = screen.getByRole("radio", { name: "Ask" })
    expect(ask).toHaveAttribute("aria-disabled", "true")
    await user.click(ask)
    expect(setMode).not.toHaveBeenCalled()
  })

  it("offers the label browser on Loki only", () => {
    setup({ type: "loki" })
    expect(screen.getByRole("button", { name: "Labels" })).toBeInTheDocument()
  })

  it("has no label browser button on ClickHouse", () => {
    setup({ type: "clickhouse" })
    expect(screen.queryByRole("button", { name: "Labels" })).toBeNull()
  })
})
