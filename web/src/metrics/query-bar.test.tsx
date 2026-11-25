import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { observeKeys } from "@/api/keys"
import type { InputMode } from "@/ask/input-mode"
import type { DatasourceStatus } from "@/datasources/queries"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { QueryBar } from "./query-bar"
import { MetricsWorkspaceContext, type MetricsWorkspace } from "./workspace"

vi.mock("@/query-editor/code-editor", () => ({
  CodeEditor: ({ value, ariaLabel }: { value: string; ariaLabel: string }) => (
    <textarea aria-label={ariaLabel} value={value} readOnly />
  ),
}))

const datasource: DatasourceStatus = {
  uid: "prom",
  type: "prometheus",
  name: "prom",
  source: "file",
  healthy: true,
  checked_at: "2026-11-19T12:00:00Z",
  dashboard_count: 0,
}

function setup({
  inputMode = "code",
  draft = "up",
  agentAvailable = true,
}: {
  inputMode?: InputMode
  draft?: string
  agentAvailable?: boolean
} = {}) {
  const run = vi.fn()
  const setInputMode = vi.fn()
  const focusQuery = vi.fn()
  const workspace = {
    datasource,
    search: {},
    mode: "graph",
    inputMode,
    setInputMode,
    effectiveQuery: undefined,
    update: vi.fn(),
    draft,
    setDraft: vi.fn(),
    run,
    range: { from: "now-1h", to: "now" },
    focusQuery,
    registerQueryFocus: vi.fn(),
  } as unknown as MetricsWorkspace
  const queryClient = testQueryClient()
  queryClient.setQueryData(observeKeys.agent(), { available: agentAvailable })
  const view = renderWithProviders(
    <MetricsWorkspaceContext value={workspace}>
      <QueryBar />
    </MetricsWorkspaceContext>,
    { queryClient, route: { path: "/", url: "/" } }
  )
  return { ...view, run, setInputMode, focusQuery }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("QueryBar", () => {
  it("offers PromQL and Ask, with PromQL checked", async () => {
    setup()
    await screen.findByRole("radiogroup", { name: "Input mode" })
    expect(
      screen.getAllByRole("radio").map((radio) => radio.textContent)
    ).toEqual(["PromQL", "Write a query"])
    expect(screen.getByRole("radio", { name: "PromQL" })).toBeChecked()
  })

  it("switches to Ask from the Ask segment and remembers the choice", async () => {
    const { setInputMode, focusQuery, user } = setup()
    await user.click(
      await screen.findByRole("radio", { name: "Write a query" })
    )
    expect(setInputMode).toHaveBeenCalledWith("ask", true)
    expect(focusQuery).toHaveBeenCalled()
  })

  it("keeps Ask disabled without a model and says why", async () => {
    const { setInputMode, user } = setup({ agentAvailable: false })
    const ask = await screen.findByRole("radio", { name: "Write a query" })
    expect(ask).toHaveAttribute("aria-disabled", "true")
    expect(ask).toHaveAccessibleDescription(
      "Add a model key to ask in plain language"
    )
    await user.click(ask)
    expect(setInputMode).not.toHaveBeenCalled()
  })

  it("shows the question field and hides the editor before the first answer in Ask", async () => {
    setup({ inputMode: "ask", draft: "" })
    expect(await screen.findByLabelText("Question")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Run query" })).toBeNull()
    expect(screen.queryByRole("button", { name: "Browse metrics" })).toBeNull()
  })

  it("runs the query from the Run query button in PromQL", async () => {
    const { run, user } = setup()
    await user.click(await screen.findByRole("button", { name: "Run query" }))
    expect(run).toHaveBeenCalledOnce()
  })

  it("does not run an empty query", async () => {
    const { run, user } = setup({ draft: " " })
    const button = await screen.findByRole("button", { name: "Run query" })
    expect(button).toBeDisabled()
    await user.click(button)
    expect(run).not.toHaveBeenCalled()
  })
})
