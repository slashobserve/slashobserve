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
const accountId = "b1f0e2d3-4c5b-4a6d-9e7f-8a9b0c1d2e3f"

describe("mentionableAgents", () => {
  it("lists the Observe Agent, then connected agents, then service accounts", () => {
    expect(
      mentionableAgents({
        observeAvailable: true,
        grants: [
          { id: grantId, client: { name: "Claude Code" } },
          { id: "g2", client: { name: null } },
        ],
        serviceAccounts: [{ id: accountId, name: "Deploy bot" }],
      })
    ).toEqual([
      observeAgent,
      { kind: "grant", id: grantId, name: "Claude Code" },
      { kind: "grant", id: "g2", name: "Unnamed MCP client" },
      { kind: "service_account", id: accountId, name: "Deploy bot" },
    ])
  })

  it("leaves the Observe Agent out while the model is off and keeps one entry per id", () => {
    expect(
      mentionableAgents({
        observeAvailable: false,
        grants: [
          { id: grantId, client: { name: "Claude Code" } },
          { id: grantId, client: { name: "Claude Code" } },
        ],
        serviceAccounts: [],
      })
    ).toEqual([{ kind: "grant", id: grantId, name: "Claude Code" }])
  })
})

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function stubServer(agents: () => Response) {
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
        case "/api/v1/me/grants":
          return json({
            grants: [
              {
                id: grantId,
                client: {
                  id: "c",
                  name: "Claude Code",
                  redirect_host: "h",
                  local: true,
                },
              },
            ],
            next_cursor: null,
          })
        default:
          return agents()
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
  it("adds the service accounts GET /agents lists", async () => {
    stubServer(() =>
      json({
        connected: [],
        service_accounts: [{ id: accountId, name: "Deploy bot" }],
      })
    )
    const { result } = renderHook(() => useMentionableAgents(), { wrapper })
    await waitFor(() =>
      expect(result.current.map((agent) => agent.name)).toEqual([
        "Observe Agent",
        "Claude Code",
        "Deploy bot",
      ])
    )
  })

  it("still offers the Observe Agent and the connected agents when GET /agents is missing", async () => {
    stubServer(() => json({ error: "not found", code: "not_found" }, 404))
    const { result } = renderHook(() => useMentionableAgents(), { wrapper })
    await waitFor(() =>
      expect(result.current.map((agent) => agent.name)).toEqual([
        "Observe Agent",
        "Claude Code",
      ])
    )
  })
})
