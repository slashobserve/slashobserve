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

describe("mentionableAgents", () => {
  it("lists the Observe Agent, then connected agents", () => {
    expect(
      mentionableAgents({
        observeAvailable: true,
        grants: [
          { id: grantId, client: { name: "Claude Code" } },
          { id: "g2", client: { name: null } },
        ],
      })
    ).toEqual([
      observeAgent,
      { kind: "grant", id: grantId, name: "Claude Code" },
      { kind: "grant", id: "g2", name: "Unnamed MCP client" },
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

function stubServer() {
  const requested: string[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request | string) => {
      const url = new URL(
        typeof request === "string" ? request : request.url,
        "http://x"
      )
      requested.push(url.pathname)
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
          return json({ error: "not found", code: "not_found" }, 404)
      }
    })
  )
  return requested
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
  it("offers the Observe Agent and the connected agents without asking for a route the server lacks", async () => {
    const requested = stubServer()
    const { result } = renderHook(() => useMentionableAgents(), { wrapper })
    await waitFor(() =>
      expect(result.current.map((agent) => agent.name)).toEqual([
        "Observe Agent",
        "Claude Code",
      ])
    )
    expect(requested).not.toContain("/api/v1/agents")
  })
})
