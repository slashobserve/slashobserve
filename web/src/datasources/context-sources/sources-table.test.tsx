import { createRef, type ComponentProps } from "react"
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
  create: vi.fn(),
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
  useCreateContextSource: () => ({
    mutate: calls.create,
    isPending: false,
    isError: false,
    error: null,
  }),
  useUpdateContextSource: () => ({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  }),
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

const noop = () => undefined

type SectionOverrides = Partial<ComponentProps<typeof McpSourcesSection>>

function section(overrides: SectionOverrides = {}) {
  const host = document.createElement("div")
  document.body.append(host)
  const props = {
    adding: false,
    addTrigger: createRef<HTMLElement>(),
    onAddMcp: noop,
    onAddClose: noop,
    onPickNative: noop,
    ...overrides,
  }
  const view = renderWithProviders(
    <PanelHostContext.Provider value={host}>
      <McpSourcesSection {...props} />
    </PanelHostContext.Provider>,
    { route: { path: "/", url: "/" } }
  )
  return { ...view, host }
}

beforeEach(() => {
  vi.clearAllMocks()
  calls.sources = [source({})]
  calls.tools = [tool({})]
})

describe("McpSourcesSection", () => {
  it("names the table and its caption and shows one row per source", async () => {
    section()
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

  it("marks a source that may use a private network address", async () => {
    calls.sources = [source({ private: true })]
    section()
    expect(
      await screen.findByText("Private network address allowed")
    ).toBeVisible()
  })

  it("keeps the tools collapsed until the chevron is used, and collapses them again", async () => {
    const { user } = section()
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
    const { user } = section()
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
    const { user } = section()
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
    section()
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

  it("has no Add source button of its own", async () => {
    section()
    await screen.findByText("Docs index")
    expect(screen.queryByRole("button", { name: "Add source" })).toBeNull()
  })

  it("offers Add an MCP server from the empty table", async () => {
    calls.sources = []
    const onAddMcp = vi.fn()
    const { user } = section({ onAddMcp })
    await user.click(
      await screen.findByRole("button", { name: "Add an MCP server" })
    )
    expect(onAddMcp).toHaveBeenCalledOnce()
  })

  it("shows the MCP form with MCP server preselected when adding", async () => {
    const { host } = section({ adding: true })
    expect(await within(host).findByText("Add a context source")).toBeVisible()
    expect(
      within(host).getByRole("combobox", { name: "Type" })
    ).toHaveTextContent("MCP server")
    expect(within(host).getByLabelText("Id")).toBeVisible()
    expect(within(host).getByLabelText("Address")).toBeVisible()
  })

  it("switches to a datasource type picked in the MCP form", async () => {
    const onPickNative = vi.fn()
    const { user, host } = section({ adding: true, onPickNative })
    await user.click(
      await within(host).findByRole("combobox", { name: "Type" })
    )
    await user.click(await screen.findByRole("option", { name: "Loki" }))
    expect(onPickNative).toHaveBeenCalledWith("loki")
  })

  it("expands a source after it is added so its tools can be approved", async () => {
    calls.sources = [source({ uid: "wiki", name: "Wiki" })]
    calls.create.mockImplementation(
      (_body: unknown, options: { onSuccess: () => void }) =>
        options.onSuccess()
    )
    const onAddClose = vi.fn()
    const { user, host } = section({ adding: true, onAddClose })
    const toggle = await screen.findByRole("button", { name: "Tools of Wiki" })
    expect(toggle).toHaveAttribute("aria-expanded", "false")
    await user.type(await within(host).findByLabelText("Id"), "wiki")
    await user.type(
      within(host).getByLabelText("Address"),
      "https://wiki.example.com/mcp"
    )
    await user.click(within(host).getByRole("button", { name: "Add source" }))
    expect(calls.create).toHaveBeenCalledOnce()
    expect(onAddClose).toHaveBeenCalledOnce()
    expect(toggle).toHaveAttribute("aria-expanded", "true")
  })
})
