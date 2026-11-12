import { QueryClientProvider } from "@tanstack/react-query"
import { renderHook, waitFor } from "@testing-library/react"
import type { ReactNode } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { testQueryClient } from "@/test/render"

import { observeAgent } from "./mentions"
import {
  mentionableAgents,
  useMentionableAgents,
} from "./use-mentionable-agents"

const grantId = "3c9a1b2d-0e4f-4a5b-8c6d-7e8f9a0b1c2d"
const accountId = "5d1e2f3a-6b7c-4d8e-9f0a-1b2c3d4e5f6a"

describe("mentionableAgents", () => {
  it("lists the Observe Agent, then connected agents, then service accounts", () => {
    expect(
      mentionableAgents({
        observeAvailable: true,
        connected: [
          { grant_id: grantId, client_name: "Claude Code" },
          { grant_id: "g2", client_name: "MCP" },
        ],
        serviceAccounts: [{ id: accountId, name: "nightly" }],
      })
    ).toEqual([
      observeAgent,
      { kind: "grant", id: grantId, name: "Claude Code" },
      { kind: "grant", id: "g2", name: "MCP" },
      { kind: "service_account", id: accountId, name: "nightly" },
    ])
  })

  it("leaves the Observe Agent out while the model is off and keeps one entry per id", () => {
    expect(
      mentionableAgents({
        observeAvailable: false,
        connected: [
          { grant_id: grantId, client_name: "Claude Code" },
          { grant_id: grantId, client_name: "Claude Code" },
        ],
        serviceAccounts: [
          { id: accountId, name: "nightly" },
          { id: accountId, name: "nightly" },
        ],
      })
    ).toEqual([
      { kind: "grant", id: grantId, name: "Claude Code" },
      { kind: "service_account", id: accountId, name: "nightly" },
    ])
  })
})

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

const stats = {
  runs: 0,
  failed: 0,
  cost_usd: null,
  median_queue_wait_seconds: null,
}

const overview = {
  observe_agent: {
    enabled: true,
    provider: "p",
    model: "m",
    runtime: "sidecar",
    scope: "you",
    last_7_days: stats,
  },
  connected: [
    {
      grant_id: grantId,
      client_name: "Claude Code",
      mode: "direct",
      present: true,
      last_seen_at: null,
      connected_at: "2026-11-01T00:00:00Z",
      writes_7d: 0,
      last_7_days: stats,
    },
  ],
  service_accounts: [
    {
      id: accountId,
      name: "nightly",
      present: false,
      last_seen_at: null,
      last_7_days: null,
    },
  ],
}

function stubServer(agents: Response = json(overview)) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request | string) => {
      const url = new URL(
        typeof request === "string" ? request : request.url,
        "http://x"
      )
      switch (url.pathname) {
        case "/api/v1/agent":
          return json({
            available: true,
            provider: "p",
            model: "m",
            effort: null,
          })
        case "/api/v1/agents":
          return agents
        default:
          return json({ error: "not found", code: "not_found" }, 404)
      }
    })
  )
}

function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={testQueryClient()}>
      {children}
    </QueryClientProvider>
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("useMentionableAgents", () => {
  it("offers the Observe Agent, the connected agents and the run-taking service accounts from GET /agents", async () => {
    stubServer()
    const { result } = renderHook(() => useMentionableAgents(), { wrapper })
    await waitFor(() =>
      expect(result.current.map((agent) => agent.name)).toEqual([
        "Observe Agent",
        "Claude Code",
        "nightly",
      ])
    )
  })

  it("keeps the Observe Agent alone when GET /agents fails", async () => {
    stubServer(json({ error: "no", code: "user_session_required" }, 403))
    const { result } = renderHook(() => useMentionableAgents(), { wrapper })
    await waitFor(() =>
      expect(result.current.map((agent) => agent.name)).toEqual([
        "Observe Agent",
      ])
    )
  })
})
