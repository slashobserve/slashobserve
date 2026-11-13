import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { QueryClient } from "@tanstack/react-query"

import { agentKeys } from "@/api/keys"
import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import { ConnectPanel } from "./connect-panel"
import type { AgentsOverview } from "./queries"

function overview(grantIds: string[]): AgentsOverview {
  return {
    observe_agent: {
      enabled: true,
      provider: null,
      model: null,
      runtime: "sidecar",
      scope: "you",
      last_7_days: {
        runs: 0,
        failed: 0,
        cost_usd: null,
        median_queue_wait_seconds: null,
      },
    },
    connected: grantIds.map((id) => ({
      grant_id: id,
      client_name: id === "new" ? "Claude Code" : "Cursor",
      mode: "direct" as const,
      present: false,
      last_seen_at: null,
      connected_at: "2026-11-11T10:00:00Z",
      writes_7d: 0,
      last_7_days: {
        runs: 0,
        failed: 0,
        cost_usd: null,
        median_queue_wait_seconds: null,
      },
    })),
    service_accounts: [],
  }
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
}

function serve(answers: string[][]) {
  let reads = 0
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url)
      if (pathname === "/api/v1/instance") {
        return json({ mcp_url: "https://observe.example/mcp" })
      }
      if (pathname === "/api/v1/agents") {
        const index = Math.min(reads, answers.length - 1)
        reads += 1
        return json(overview(answers[index]))
      }
      return json({})
    })
  )
}

function renderPanel(onClose = vi.fn()) {
  const host = document.createElement("div")
  document.body.append(host)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const view = renderWithProviders(
    <PanelHostContext value={host}>
      <ConnectPanel onClose={onClose} />
    </PanelHostContext>,
    { queryClient }
  )
  return { ...view, onClose, queryClient }
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
  document.body.replaceChildren()
})

describe("ConnectPanel", () => {
  it("shows the Claude Code command for this server and waits for the connection", async () => {
    serve([["old"]])
    renderPanel()

    expect(
      await screen.findByRole("region", { name: "Claude Code connect command" })
    ).toHaveTextContent(
      "claude mcp add --transport http slashobserve https://observe.example/mcp"
    )
    expect(
      screen.getByText("Approve the connection in the browser window it opens.")
    ).toBeTruthy()
    const waiting = await screen.findByText(
      "Waiting for Claude Code to connect"
    )
    expect(waiting.closest("[role=status]")).not.toBeNull()
  })

  it("says Connected once a new grant shows up, and Done closes the panel", async () => {
    serve([["old"], ["old", "new"]])
    const { user, onClose, queryClient } = renderPanel()

    await waitFor(() =>
      expect(queryClient.getQueryData(agentKeys.list())).toBeDefined()
    )
    await queryClient.refetchQueries({ queryKey: ["agents"] })

    const connected = await screen.findByText("Connected: Claude Code")
    expect(connected.closest("[role=status]")).not.toBeNull()
    expect(screen.queryByText("Waiting for Claude Code to connect")).toBeNull()
    await user.click(screen.getByRole("button", { name: "Done" }))
    expect(onClose).toHaveBeenCalled()
  })

  it("does not call a grant that was already there a new connection", async () => {
    serve([["old"], ["old"]])
    const { queryClient } = renderPanel()

    await waitFor(() =>
      expect(queryClient.getQueryData(agentKeys.list())).toBeDefined()
    )
    await queryClient.refetchQueries({ queryKey: ["agents"] })

    expect(screen.getByText("Waiting for Claude Code to connect")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Done" })).toBeNull()
  })
})
