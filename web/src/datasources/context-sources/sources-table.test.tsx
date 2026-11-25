import { queryOptions } from "@tanstack/react-query"
import { screen, waitFor, within } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import type { ContextSource, ContextTool } from "./queries"
import { McpSourcesSection } from "./sources-table"

const calls = vi.hoisted(() => ({
  approve: vi.fn(),
  revoke: vi.fn(),
  discover: vi.fn(),
  sources: [] as unknown[],
  tools: [] as unknown[],
}))

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
vi.mock("./queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./queries")>()),
  contextSourcesQuery: () =>
    queryOptions({
      queryKey: ["sources"],
      queryFn: () => Promise.resolve(calls.sources as ContextSource[]),
    }),
  contextToolsQuery: (uid: string) =>
    queryOptions({
      queryKey: ["tools", uid],
      queryFn: () => Promise.resolve(calls.tools as ContextTool[]),
    }),
  useApproveContextTool: () => ({ mutate: calls.approve, isPending: false }),
  useApproveReadOnlyTools: () => ({ mutate: vi.fn(), isPending: false }),
  useRevokeContextTool: () => ({ mutate: calls.revoke, isPending: false }),
  useDiscoverContextSource: () => ({
    mutate: calls.discover,
    isPending: false,
  }),
  useDeleteContextSource: () => ({
    mutateAsync: vi.fn(),
    reset: vi.fn(),
    isPending: false,
    isError: false,
  }),
}))

function source(overrides: Partial<ContextSource>): ContextSource {
  return {
    uid: "docs",
    name: "Docs index",
    description: null,
    url: "https://docs.example.com/mcp",
    audience: "everyone",
    secret_set: false,
    secret_header: null,
    provisioned: false,
    private: false,
    state: "ready",
    failure: null,
    checked_at: "2026-11-25T10:00:00Z",
    tools: { approved: 2, offered: 1, changed: 0, gone: 0, refused: 0 },
    ...overrides,
  } as ContextSource
}

function tool(overrides: Partial<ContextTool>): ContextTool {
  return {
    name: "search",
    state: "offered",
    refusal: null,
    description: "Search the docs",
    input_schema: {},
    hash: "a".repeat(64),
    seen: null,
    approved_by: null,
    approved_at: null,
    updated_at: "2026-11-25T10:00:00Z",
    allowed_actions: ["approve"],
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  calls.sources = [source({})]
  calls.tools = [tool({})]
})

describe("McpSourcesSection", () => {
  it("names the table and its caption and shows one row per source", async () => {
    renderWithProviders(<McpSourcesSection />)
    expect(
      await screen.findByRole("heading", { name: "MCP sources" })
    ).toBeVisible()
    expect(
      screen.getByText("MCP servers the agent may read from")
    ).toBeVisible()
    const row = (await screen.findByText("Docs index")).closest("[data-uid]")
    expect(row).toHaveTextContent("docs")
    expect(row).toHaveTextContent("https://docs.example.com/mcp")
    expect(row).toHaveTextContent("Everyone")
    expect(row).toHaveTextContent("Ready")
    expect(row).toHaveTextContent("2")
  })

  it("keeps the tools collapsed until the chevron is used, and collapses them again", async () => {
    const { user } = renderWithProviders(<McpSourcesSection />)
    const toggle = await screen.findByRole("button", {
      name: "Tools of Docs index",
    })
    expect(toggle).toHaveAttribute("aria-expanded", "false")
    expect(screen.queryByText("Search the docs")).toBeNull()

    await user.click(toggle)
    expect(toggle).toHaveAttribute("aria-expanded", "true")
    expect(await screen.findByText("Search the docs")).toBeVisible()
    expect(screen.getByText("Offered")).toBeVisible()

    await user.click(toggle)
    expect(toggle).toHaveAttribute("aria-expanded", "false")
    await waitFor(() =>
      expect(screen.queryByText("Search the docs")).toBeNull()
    )
  })

  it("expands with the keyboard", async () => {
    const { user } = renderWithProviders(<McpSourcesSection />)
    const toggle = await screen.findByRole("button", {
      name: "Tools of Docs index",
    })
    toggle.focus()
    await user.keyboard("{Enter}")
    expect(toggle).toHaveAttribute("aria-expanded", "true")
    expect(await screen.findByText("Search the docs")).toBeVisible()
  })

  it("approves and revokes a tool from its nested row", async () => {
    calls.tools = [
      tool({}),
      tool({
        name: "fetch",
        state: "approved",
        allowed_actions: ["revoke"],
      }),
    ]
    const { user } = renderWithProviders(<McpSourcesSection />)
    await user.click(
      await screen.findByRole("button", { name: "Tools of Docs index" })
    )
    await user.click(
      await screen.findByRole("button", { name: "Approve search" })
    )
    expect(calls.approve.mock.calls[0][0]).toMatchObject({
      uid: "docs",
      tool: { name: "search" },
    })
    await user.click(screen.getByRole("button", { name: "Revoke fetch" }))
    expect(calls.revoke.mock.calls[0][0]).toEqual({
      uid: "docs",
      name: "fetch",
    })
  })

  it("disables Edit and Delete for a source from the file but not Discover", async () => {
    calls.sources = [source({ provisioned: true })]
    renderWithProviders(<McpSourcesSection />)
    expect(
      await screen.findByRole("button", { name: "Edit Docs index" })
    ).toBeDisabled()
    expect(
      screen.getByRole("button", { name: "Delete Docs index" })
    ).toBeDisabled()
    expect(
      screen.getByRole("button", { name: "Discover the tools of Docs index" })
    ).toBeEnabled()
  })

  it("opens the add panel from the table header", async () => {
    const host = document.createElement("div")
    document.body.append(host)
    const { user } = renderWithProviders(
      <PanelHostContext.Provider value={host}>
        <McpSourcesSection />
      </PanelHostContext.Provider>,
      { route: { path: "/", url: "/" } }
    )
    await user.click(await screen.findByRole("button", { name: "Add source" }))
    expect(await within(host).findByText("Add a context source")).toBeVisible()
    host.remove()
  })
})
