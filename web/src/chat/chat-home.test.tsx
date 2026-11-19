import { screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { dashboardKeys, datasourceKeys, observeKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { ChatHome } from "./chat-home"

const navigate = vi.hoisted(() => vi.fn())

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useNavigate: () => navigate,
  useRouteContext: () => ({ principal: { id: "u1", role: "viewer" } }),
}))
vi.mock("@/home/dashboards-section", () => ({ DashboardsSection: () => null }))
vi.mock("@/prompts/saved-prompts", () => ({ SavedPrompts: () => null }))
vi.mock("./recent-chats", () => ({ RecentChats: () => null }))

const mine = "2f0f2c1e-6a53-4a35-8a0e-1d3b0d1f7a11"
const theirs = "9b1c5a42-1c9e-4d5e-9a67-3f8e2d1c0b44"

function dashboard(id: string, title: string, edit: boolean) {
  return { id, title, permissions: { comment: true, edit } }
}

function render(
  props: { initialPrompt?: string; dashboardId?: string } = {},
  withTitles = true
) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(observeKeys.agent(), {
    available: true,
    provider: "anthropic",
    model: "claude-haiku-5-5",
    effort: "medium",
  })
  queryClient.setQueryData(datasourceKeys.list(), {
    datasources: [{ uid: "prom", type: "prometheus", healthy: true }],
  })
  if (withTitles) {
    queryClient.setQueryData(dashboardKeys.titles(), {
      dashboards: [
        dashboard(theirs, "Someone else's board", false),
        dashboard(mine, "Payments", true),
      ],
    })
  }
  return renderWithProviders(<ChatHome {...props} />, {
    queryClient,
    route: { path: "/", url: "/" },
  })
}

const fetch = vi.fn<(request: Request) => Promise<Response>>(
  async () =>
    new Response("[]", {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })
)

beforeEach(() => vi.stubGlobal("fetch", fetch))

afterEach(() => {
  navigate.mockReset()
  fetch.mockClear()
  vi.unstubAllGlobals()
})

describe("ChatHome", () => {
  it("offers a revision only of a dashboard the person can edit", async () => {
    render()
    expect(
      await screen.findByRole("button", {
        name: "Add a latency row to Payments",
      })
    ).toBeInTheDocument()
    expect(screen.queryByText(/Someone else's board/)).not.toBeInTheDocument()
  })

  it("fills the box through the address instead of sending the revision", async () => {
    const { user } = render()
    await user.click(
      await screen.findByRole("button", {
        name: "Add a latency row to Payments",
      })
    )
    expect(navigate).toHaveBeenCalledWith({
      to: "/ask",
      search: { prompt: "Add a latency row to Payments", dashboard: mine },
    })
    const posts = fetch.mock.calls.filter(
      ([request]) => request.method === "POST"
    )
    expect(posts).toEqual([])
  })

  it("shows the dashboard to revise as a chip that can be removed", async () => {
    const { user } = render({
      initialPrompt: "Add a latency row to Payments",
      dashboardId: mine,
    })
    expect(
      await screen.findByRole("textbox", { name: "Message the agent" })
    ).toHaveValue("Add a latency row to Payments")
    const chips = screen.getByRole("list", { name: "Context" })
    expect(chips).toHaveTextContent("Payments")
    await user.click(screen.getByRole("button", { name: "Remove Payments" }))
    expect(navigate).toHaveBeenCalledWith({
      to: "/ask",
      search: { prompt: "Add a latency row to Payments" },
      replace: true,
    })
  })

  it("ignores a dashboard the person cannot edit", async () => {
    render({ initialPrompt: "Add a row", dashboardId: theirs })
    await screen.findByRole("textbox", { name: "Message the agent" })
    expect(
      screen.queryByRole("list", { name: "Context" })
    ).not.toBeInTheDocument()
  })

  it("looks up a dashboard to revise that the title list does not hold", async () => {
    const far = "5c1d9a40-7a3b-4e2f-8c55-0d9e6b2a1f33"
    fetch.mockImplementation(async (request) => {
      const path = new URL(request.url).pathname
      const body = path.endsWith(`/dashboards/${far}`)
        ? {
            dashboard: dashboard(far, "Archive", true),
            version: { version: 1 },
          }
        : { dashboards: [dashboard(mine, "Payments", true)] }
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    })
    render({ initialPrompt: "Add a row", dashboardId: far })
    const chips = await screen.findByRole("list", { name: "Context" })
    expect(chips).toHaveTextContent("Archive")
  })

  it("keeps the box closed while the dashboard to revise is being found", async () => {
    fetch.mockImplementation(() => new Promise<Response>(() => undefined))
    render({ initialPrompt: "Add a row", dashboardId: mine }, false)
    expect(
      await screen.findByRole("textbox", { name: "Message the agent" })
    ).toBeDisabled()
  })
})
