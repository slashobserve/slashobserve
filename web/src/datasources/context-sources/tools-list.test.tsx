import { queryOptions } from "@tanstack/react-query"
import { screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import type { ContextTool } from "./queries"
import { ToolsList } from "./tools-list"

const calls = vi.hoisted(() => ({
  approve: vi.fn(),
  approveAll: vi.fn(),
  revoke: vi.fn(),
  tools: [] as unknown[],
}))

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
vi.mock("./queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./queries")>()),
  contextToolsQuery: () =>
    queryOptions({
      queryKey: ["tools"],
      queryFn: () => Promise.resolve(calls.tools as ContextTool[]),
    }),
  useApproveContextTool: () => ({ mutate: calls.approve, isPending: false }),
  useApproveReadOnlyTools: () => ({
    mutate: calls.approveAll,
    isPending: false,
  }),
  useRevokeContextTool: () => ({ mutate: calls.revoke, isPending: false }),
}))

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
    expect(calls.approve.mock.calls[0][0]).toMatchObject({
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
    expect(calls.approve.mock.calls[0][0].tool.seen.hash).toBe("b".repeat(64))
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

  it("approves every read-only tool at once only while one is waiting", async () => {
    calls.tools = [tool({}), tool({ name: "list" })]
    const { user } = renderWithProviders(<ToolsList uid="docs" />)
    await user.click(
      await screen.findByRole("button", { name: "Approve all read-only" })
    )
    expect(calls.approveAll.mock.calls[0][0]).toBe("docs")
  })

  it("hides the bulk button when nothing waits for review", async () => {
    calls.tools = [tool({ state: "approved", allowed_actions: ["revoke"] })]
    const { user } = renderWithProviders(<ToolsList uid="docs" />)
    await user.click(
      await screen.findByRole("button", { name: "Revoke search" })
    )
    expect(calls.revoke.mock.calls[0][0]).toEqual({
      uid: "docs",
      name: "search",
    })
    expect(
      screen.queryByRole("button", { name: "Approve all read-only" })
    ).toBeNull()
  })
})
