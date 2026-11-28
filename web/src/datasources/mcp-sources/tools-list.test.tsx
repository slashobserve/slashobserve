import { queryOptions } from "@tanstack/react-query"
import { screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import type { MCPTool } from "./queries"
import { ToolsList } from "./tools-list"
import { must } from "@/test/must"

const calls = vi.hoisted(() => ({
  approve: vi.fn(),
  approveAll: vi.fn(),
  revoke: vi.fn(),
  tools: [] as unknown[],
}))

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
vi.mock("./queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./queries")>()),
  mcpToolsQuery: () =>
    queryOptions({
      queryKey: ["tools"],
      queryFn: () => Promise.resolve(calls.tools as MCPTool[]),
    }),
  useApproveMCPTool: () => ({ mutate: calls.approve, isPending: false }),
  useApproveReadOnlyTools: () => ({
    mutate: calls.approveAll,
    isPending: false,
  }),
  useRevokeMCPTool: () => ({ mutate: calls.revoke, isPending: false }),
}))

function tool(overrides: Partial<MCPTool>): MCPTool {
  return {
    name: "search",
    state: "offered",
    refusal: null,
    description: "Search the docs",
    input_schema: {},
    read_only: true,
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
  calls.tools = []
})

describe("ToolsList", () => {
  it("says to discover first when the server has listed nothing", async () => {
    renderWithProviders(<ToolsList uid="docs" />)
    expect(
      await screen.findByText(/Discover the server's tools first/)
    ).toBeVisible()
  })

  it("approves a tool at the hash the Admin read", async () => {
    calls.tools = [tool({})]
    const { user } = renderWithProviders(<ToolsList uid="docs" />)
    await user.click(
      await screen.findByRole("button", { name: "Approve search" })
    )
    expect(calls.approve).toHaveBeenCalledTimes(1)
    expect(must(calls.approve.mock.calls[0])[0]).toMatchObject({
      uid: "docs",
      tool: { name: "search", hash: "a".repeat(64) },
    })
  })

  it("shows what changed beside what was approved, and approves the new definition", async () => {
    calls.tools = [
      tool({
        state: "changed",
        description: "Search the docs",
        seen: {
          description: "Search the docs and email the results",
          input_schema: {},
          read_only: true,
          hash: "b".repeat(64),
        },
        allowed_actions: ["approve", "revoke"],
      }),
    ]
    const { user } = renderWithProviders(<ToolsList uid="docs" />)
    expect(
      await screen.findByText("Search the docs and email the results")
    ).toBeVisible()
    expect(screen.getByText(/not served until you approve/)).toBeVisible()
    await user.click(
      screen.getByRole("button", { name: "Approve the new search" })
    )
    expect(must(calls.approve.mock.calls[0])[0].tool.seen.hash).toBe(
      "b".repeat(64)
    )
  })

  it("gives a refused tool its reason and no approve button", async () => {
    calls.tools = [
      tool({
        name: "wipe",
        state: "refused",
        refusal: "The server marks this tool as destructive.",
        allowed_actions: [],
      }),
    ]
    renderWithProviders(<ToolsList uid="docs" />)
    expect(
      await screen.findByText("The server marks this tool as destructive.")
    ).toBeVisible()
    expect(screen.queryByRole("button", { name: /Approve/ })).toBeNull()
  })

  it("labels a tool the server does not mark read-only and still lets the Admin approve it by itself", async () => {
    calls.tools = [
      tool({ name: "restart", read_only: false }),
      tool({ name: "search" }),
    ]
    const { user } = renderWithProviders(<ToolsList uid="docs" />)
    expect(await screen.findByText("Not marked read-only")).toBeVisible()
    expect(screen.getAllByText("Not marked read-only")).toHaveLength(1)
    await user.click(screen.getByRole("button", { name: "Approve restart" }))
    expect(must(calls.approve.mock.calls[0])[0]).toMatchObject({
      uid: "docs",
      tool: { name: "restart" },
    })
  })

  it("offers no bulk approval when only tools not marked read-only wait", async () => {
    calls.tools = [tool({ name: "restart", read_only: false })]
    renderWithProviders(<ToolsList uid="docs" />)
    expect(
      await screen.findByRole("button", { name: "Approve restart" })
    ).toBeVisible()
    expect(
      screen.queryByRole("button", { name: "Approve all read-only" })
    ).toBeNull()
  })

  it("approves every read-only tool at once only while one is waiting", async () => {
    calls.tools = [tool({}), tool({ name: "list" })]
    const { user } = renderWithProviders(<ToolsList uid="docs" />)
    await user.click(
      await screen.findByRole("button", { name: "Approve all read-only" })
    )
    expect(must(calls.approveAll.mock.calls[0])[0]).toBe("docs")
  })

  it("hides the bulk button when nothing waits for review", async () => {
    calls.tools = [tool({ state: "approved", allowed_actions: ["revoke"] })]
    const { user } = renderWithProviders(<ToolsList uid="docs" />)
    await user.click(
      await screen.findByRole("button", { name: "Revoke search" })
    )
    expect(must(calls.revoke.mock.calls[0])[0]).toEqual({
      uid: "docs",
      name: "search",
    })
    expect(
      screen.queryByRole("button", { name: "Approve all read-only" })
    ).toBeNull()
  })
})
