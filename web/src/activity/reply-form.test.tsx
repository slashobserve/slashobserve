import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import type { AgentGate } from "./agent-gate"
import { ReplyForm } from "./reply-form"

const rootId = "9c0f7a2e-1b3d-4e5f-8a6b-7c8d9e0f1a2b"
const runId = "6f1e2d3c-4b5a-4968-8776-655443322110"

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function stubServer(sent: Record<string, unknown> = {}) {
  const posts: { path: string; body: Record<string, unknown> }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url)
      if (request.method === "POST") {
        posts.push({ path: pathname, body: await request.clone().json() })
        return json(
          { thread: { id: rootId }, message: { id: "m2", ...sent } },
          201
        )
      }
      if (pathname === "/api/v1/agent") {
        return json({
          available: true,
          provider: "p",
          model: "m",
          effort: null,
        })
      }
      if (pathname === "/api/v1/me/grants") {
        return json({ grants: [], next_cursor: null })
      }
      return json({ service_accounts: [] })
    })
  )
  return posts
}

afterEach(() => {
  vi.unstubAllGlobals()
})

async function renderReply(gate: AgentGate = { kind: "allowed" }) {
  const onClose = vi.fn()
  const view = renderWithProviders(
    <ReplyForm
      dashboardId="dash"
      rootId={rootId}
      people={[]}
      gate={gate}
      latestVersion={3}
      onClose={onClose}
    />,
    { route: { path: "/", url: "/" } }
  )
  const field = await screen.findByRole("textbox", { name: "Reply" })
  return { ...view, onClose, field }
}

describe("ReplyForm", () => {
  beforeEach(() => {
    stubServer()
  })

  it("replies in the thread without start_runs and closes", async () => {
    const posts = stubServer()
    const { user, field, onClose } = await renderReply()
    await user.type(field, "Thanks")
    await user.click(screen.getByRole("button", { name: "Send" }))
    await waitFor(() => expect(onClose).toHaveBeenCalledOnce())
    expect(posts).toEqual([
      { path: `/api/v1/threads/${rootId}/messages`, body: { body: "Thanks" } },
    ])
  })

  it("starts a run for a mentioned agent and stays open with the run line", async () => {
    const posts = stubServer({
      run: { id: runId, status: "queued", assigned_kind: "builtin" },
    })
    const { user, field, onClose } = await renderReply()
    await user.click(field)
    await user.paste("@observe check this")
    await user.click(
      screen.getByRole("button", { name: "Send and ask Observe Agent" })
    )
    expect(
      await screen.findByText(/Observe Agent is working on it/)
    ).toBeVisible()
    expect(posts[0].body).toEqual({
      body: "@observe check this",
      start_runs: true,
    })
    expect(onClose).not.toHaveBeenCalled()
    await user.click(screen.getByRole("button", { name: "Done" }))
    expect(onClose).toHaveBeenCalledOnce()
  })

  it("has no Send to agent button and Cancel closes", async () => {
    const { user, onClose } = await renderReply()
    expect(screen.queryByRole("button", { name: "Send to agent" })).toBeNull()
    await user.click(screen.getByRole("button", { name: "Cancel" }))
    expect(onClose).toHaveBeenCalledOnce()
  })

  it("holds an agent mention back and says why while the agent is blocked", async () => {
    const reason = "Finish editing the layout to send changes to the agent."
    const { user, field } = await renderReply({ kind: "blocked", reason })
    await user.click(field)
    await user.paste("@observe go")
    expect(screen.getAllByText(reason).length).toBeGreaterThan(0)
    expect(
      screen.getByRole("button", { name: "Send and ask Observe Agent" })
    ).toBeDisabled()
  })
})
