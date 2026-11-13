import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AgentDetailPage } from "./agent-detail-page"
import type { AgentRef } from "./agent-ref"
import type { AgentsOverview } from "./queries"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({
    principal: { id: "u1", name: "Ada Lovelace", role: "viewer" },
  }),
}))

const stats = {
  runs: 14,
  failed: 2,
  cost_usd: 1.5,
  median_queue_wait_seconds: 180,
}

const overview: AgentsOverview = {
  observe_agent: {
    enabled: true,
    provider: "anthropic",
    model: "claude-haiku-5-5",
    runtime: "sidecar",
    scope: "you",
    last_7_days: stats,
  },
  connected: [
    {
      grant_id: "g1",
      client_name: "Claude Code",
      mode: "propose",
      present: false,
      presence: "offline",
      last_seen_at: null,
      connected_at: "2026-11-01T10:00:00Z",
      writes_7d: 7,
      last_7_days: { ...stats, runs: 3 },
    },
  ],
  service_accounts: [
    {
      id: "s1",
      name: "nightly-audit",
      present: true,
      presence: "listening",
      last_seen_at: null,
      last_7_days: null,
    },
  ],
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function serve(reply: () => Response = () => json(overview)) {
  const fetch = vi.fn(async (request: Request) =>
    new URL(request.url).pathname === "/api/v1/agents"
      ? reply()
      : json({ error: "unexpected", code: "unexpected" }, 500)
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

function renderDetail(agentRef: AgentRef | null) {
  return renderWithProviders(<AgentDetailPage agentRef={agentRef} />, {
    route: { path: "/", url: "/" },
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("AgentDetailPage", () => {
  it("shows a connected agent's name, who it acts for, its mode and its numbers", async () => {
    serve()
    renderDetail({ kind: "grant", id: "g1" })

    expect(
      await screen.findByRole("heading", { level: 1, name: "Claude Code" })
    ).toBeTruthy()
    expect(screen.getByText("Acts for Ada Lovelace")).toBeTruthy()
    expect(
      screen.getByRole("combobox", { name: "Mode for Claude Code" })
    ).toBeTruthy()
    expect(screen.getByText("Never seen")).toBeTruthy()
    expect(screen.getByText("Runs").nextElementSibling?.textContent).toBe("3")
    expect(screen.getByText("Cost").nextElementSibling?.textContent).toBe(
      "$1.50"
    )
    expect(
      screen.getByText("Median wait").nextElementSibling?.textContent
    ).toBe("3 min")
    expect(
      screen.getByText("Writes in 7 days").nextElementSibling?.textContent
    ).toBe("7")
    expect(screen.getByText("Connected")).toBeTruthy()
    expect(
      screen.getByRole("heading", { level: 2, name: "Recent work" })
    ).toBeTruthy()
  })

  it("shows Observe Agent for anyone who asks, with the scope of its numbers", async () => {
    serve()
    renderDetail({ kind: "observe" })

    expect(
      await screen.findByRole("heading", { level: 1, name: "Observe Agent" })
    ).toBeTruthy()
    expect(screen.getByText("Runs for anyone who asks")).toBeTruthy()
    expect(screen.getByText("On")).toBeTruthy()
    expect(screen.getByText("Last 7 days, yours")).toBeTruthy()
    expect(screen.queryByRole("combobox")).toBeNull()
  })

  it("shows a service account with no numbers when the caller may not see them", async () => {
    serve()
    renderDetail({ kind: "service_account", id: "s1" })

    expect(
      await screen.findByRole("heading", { level: 1, name: "nightly-audit" })
    ).toBeTruthy()
    expect(screen.getByText("Service account")).toBeTruthy()
    expect(screen.getByText("Listening")).toBeTruthy()
    expect(screen.queryByText("Runs")).toBeNull()
  })

  it("says the agent is not there when the grant was disconnected", async () => {
    serve()
    renderDetail({ kind: "grant", id: "gone" })

    expect(
      await screen.findByText(
        "This agent does not exist or is not yours to see."
      )
    ).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "Back to Agents" }).getAttribute("href")
    ).toBe("/agents")
  })

  it("says so without asking the server when the address is not an agent", async () => {
    const fetch = serve()
    renderDetail(null)

    expect(
      await screen.findByText(
        "This agent does not exist or is not yours to see."
      )
    ).toBeTruthy()
    expect(fetch).not.toHaveBeenCalled()
  })

  it("offers a retry when the agents cannot be loaded", async () => {
    serve(() => json({ error: "boom", code: "internal" }, 500))
    renderDetail({ kind: "observe" })

    expect(await screen.findByText("Could not load this agent")).toBeTruthy()
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy()
  })
})
