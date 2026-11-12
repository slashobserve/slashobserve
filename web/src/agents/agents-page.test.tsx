import { screen, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AgentsPage } from "./agents-page"
import type { AgentsOverview } from "./queries"

const session = vi.hoisted(() => ({ role: "admin" }))
vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: session.role } }),
}))

const stats = {
  runs: 14,
  failed: 2,
  cost_usd: 1.5,
  median_queue_wait_seconds: 12,
}

function overview(over: Partial<AgentsOverview> = {}): AgentsOverview {
  return {
    observe_agent: {
      enabled: true,
      provider: "anthropic",
      model: "claude-haiku-5-5",
      runtime: "go",
      scope: "workspace",
      last_7_days: stats,
    },
    connected: [
      {
        grant_id: "g1",
        client_name: "Claude Code",
        mode: "propose",
        present: true,
        last_seen_at: "2026-11-11T10:00:00Z",
        connected_at: "2026-11-01T10:00:00Z",
        writes_7d: 7,
        last_7_days: { ...stats, runs: 3 },
      },
    ],
    service_accounts: [
      {
        id: "s1",
        name: "nightly-audit",
        present: false,
        last_seen_at: null,
        last_7_days: { ...stats, runs: 21, failed: 4 },
      },
    ],
    ...over,
  }
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function serve(reply: () => Response) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) =>
      new URL(request.url).pathname === "/api/v1/agents"
        ? reply()
        : json({ error: "unexpected", code: "unexpected" }, 500)
    )
  )
}

function renderPage(onConnectingChange = vi.fn()) {
  const view = renderWithProviders(
    <AgentsPage connecting={false} onConnectingChange={onConnectingChange} />,
    { route: { path: "/", url: "/" } }
  )
  return { ...view, onConnectingChange }
}

afterEach(() => {
  vi.unstubAllGlobals()
  session.role = "admin"
})

describe("AgentsPage", () => {
  it("shows Observe Agent, the connected agents and the service accounts with their numbers for an Admin", async () => {
    serve(() => json(overview()))
    renderPage()

    const card = await screen.findByRole("region", { name: "Observe Agent" })
    expect(within(card).getByText("On")).toBeTruthy()
    expect(within(card).getByText("anthropic claude-haiku-5-5")).toBeTruthy()
    expect(within(card).getByText("14")).toBeTruthy()
    expect(within(card).getByText("$1.50")).toBeTruthy()
    expect(within(card).getByText("12 s")).toBeTruthy()
    expect(within(card).getByText("Last 7 days, everyone")).toBeTruthy()

    const connected = screen.getByRole("region", { name: "Connected agents" })
    expect(within(connected).getByText("Claude Code")).toBeTruthy()
    expect(within(connected).getByText("Present")).toBeTruthy()
    expect(within(connected).getByText("7")).toBeTruthy()
    expect(
      within(connected).getByRole("combobox", { name: "Mode for Claude Code" })
    ).toBeTruthy()
    expect(
      within(connected).getByRole("link", {
        name: "Manage Claude Code in Settings",
      })
    ).toBeTruthy()

    const accounts = screen.getByRole("region", {
      name: "Service accounts that take runs",
    })
    expect(within(accounts).getByText("nightly-audit")).toBeTruthy()
    expect(within(accounts).getByText("Never seen")).toBeTruthy()
    expect(within(accounts).getByText("21")).toBeTruthy()
    expect(screen.getByRole("link", { name: "API keys" })).toBeTruthy()
  })

  it("hides the service accounts' numbers and the keys link from a member", async () => {
    session.role = "viewer"
    serve(() =>
      json(
        overview({
          observe_agent: {
            ...overview().observe_agent,
            scope: "you",
          },
          service_accounts: [
            {
              id: "s1",
              name: "nightly-audit",
              present: true,
              last_seen_at: null,
              last_7_days: null,
            },
          ],
        })
      )
    )
    renderPage()

    const accounts = await screen.findByRole("region", {
      name: "Service accounts that take runs",
    })
    expect(within(accounts).getByText("nightly-audit")).toBeTruthy()
    expect(within(accounts).queryByText("Runs in 7 days")).toBeNull()
    expect(screen.queryByRole("link", { name: "API keys" })).toBeNull()
    expect(screen.getByText("Last 7 days, yours")).toBeTruthy()
  })

  it("says so when nothing is connected and nothing takes runs, and offers Connect", async () => {
    serve(() => json(overview({ connected: [], service_accounts: [] })))
    const { user, onConnectingChange } = renderPage()

    expect(await screen.findByText("No connected agents yet.")).toBeTruthy()
    expect(screen.getByText("No service accounts take runs.")).toBeTruthy()

    const buttons = screen.getAllByRole("button", { name: "Connect an agent" })
    expect(buttons).toHaveLength(2)
    await user.click(buttons[1])
    expect(onConnectingChange).toHaveBeenCalledWith(true)
  })

  it("shows Off and no model while the agent is not configured", async () => {
    serve(() =>
      json(
        overview({
          observe_agent: {
            ...overview().observe_agent,
            enabled: false,
            provider: null,
            model: null,
          },
        })
      )
    )
    renderPage()

    const card = await screen.findByRole("region", { name: "Observe Agent" })
    expect(within(card).getByText("Off")).toBeTruthy()
    expect(within(card).getByText("No model configured")).toBeTruthy()
  })

  it("tells a key session the page is for signed-in people", async () => {
    serve(() => json({ error: "no", code: "user_session_required" }, 403))
    renderPage()

    expect(
      await screen.findByText("The Agents page is for signed-in people.")
    ).toBeTruthy()
  })

  it("offers a retry when the agents cannot be loaded", async () => {
    serve(() => json({ error: "boom", code: "internal" }, 500))
    renderPage()

    expect(await screen.findByText("Could not load the agents")).toBeTruthy()
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy()
  })
})
