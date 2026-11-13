import { QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders, testQueryClient } from "@/test/render"

import { ActivityComposer, type Target } from "./activity-composer"
import { useActivityStore } from "./activity-store"
import type { AgentGate } from "./agent-gate"

vi.mock("@/chat/prompt-history", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/chat/prompt-history")>()),
  usePromptHistory: () => ({ entries: [], record: vi.fn() }),
}))

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

const grantId = "3c9a1b2d-0e4f-4a5b-8c6d-7e8f9a0b1c2d"

function stubServer(
  sent: Record<string, unknown> = {},
  postHeld: Promise<void> = Promise.resolve()
) {
  const posts: { path: string; body: Record<string, unknown> }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url)
      if (request.method === "POST") {
        posts.push({ path: pathname, body: await request.clone().json() })
        await postHeld
        return json(
          {
            thread: { id: "t1" },
            message: { id: "t1", thread_id: "t1", body: "", ...sent },
          },
          201
        )
      }
      if (pathname === "/api/v1/agent") {
        return json({
          available: true,
          provider: "anthropic",
          model: "claude-haiku-5-5",
          effort: null,
        })
      }
      if (pathname === "/api/v1/agents") {
        return json({
          connected: [{ grant_id: grantId, client_name: "Claude Code" }],
          service_accounts: [],
        })
      }
      return json({ service_accounts: [] })
    })
  )
  return posts
}

afterEach(() => {
  vi.unstubAllGlobals()
})

const panels = [
  { id: "latency", title: "Latency" },
  { id: "errors", title: "Errors" },
]

async function renderComposer({
  gate = { kind: "allowed" },
  draft = "Raise the threshold",
  canEdit = true,
  target = { kind: "dashboard" },
}: {
  gate?: AgentGate
  draft?: string
  canEdit?: boolean
  target?: Target
} = {}) {
  const onTargetChange = vi.fn<(target: Target) => void>()
  renderWithProviders(
    <ActivityComposer
      dashboardId="dashboard"
      latestVersion={3}
      panels={panels}
      target={target}
      onTargetChange={onTargetChange}
      draft={draft}
      onDraftChange={vi.fn()}
      gate={gate}
      canEdit={canEdit}
      placeholder="Write a comment"
      range={{ from: "now-1h", to: "now" }}
    />,
    { route: { path: "/", url: "/" } }
  )
  return {
    form: await screen.findByRole("form", { name: "Add to the activity" }),
    onTargetChange,
  }
}

describe("ActivityComposer", () => {
  beforeEach(() => {
    useActivityStore.getState().setAgentKind("revise")
    stubServer()
  })

  it("holds the target combobox, starting with the whole dashboard, and the mention picker", async () => {
    const user = userEvent.setup()
    const { form } = await renderComposer()
    const comboboxes = within(form).getAllByRole("combobox")
    expect(comboboxes).toHaveLength(2)
    expect(comboboxes[0]).toHaveAccessibleName("On")
    expect(comboboxes[0]).toHaveTextContent("The whole dashboard")
    expect(comboboxes[1]).toHaveAccessibleName("Mention a person or an agent")
    await user.click(comboboxes[0])
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual([
      "The whole dashboard",
      "Latency",
      "Errors",
    ])
  })

  it("targets a panel when one is chosen", async () => {
    const user = userEvent.setup()
    const { form, onTargetChange } = await renderComposer()
    await user.click(within(form).getByRole("combobox", { name: "On" }))
    await user.click(await screen.findByRole("option", { name: "Errors" }))
    expect(onTargetChange).toHaveBeenCalledWith({
      kind: "panel",
      panelId: "errors",
    })
  })

  it("has one send button and no separate agent button", async () => {
    const { form } = await renderComposer()
    expect(within(form).getByRole("button", { name: "Send" })).toBeEnabled()
    expect(
      within(form).queryByRole("button", { name: "Add comment" })
    ).not.toBeInTheDocument()
    expect(
      within(form).queryByRole("button", { name: "Send to agent" })
    ).not.toBeInTheDocument()
  })

  it("posts a plain comment to the dashboard thread without start_runs", async () => {
    const posts = stubServer()
    const user = userEvent.setup()
    const { form } = await renderComposer({
      target: { kind: "panel", panelId: "latency" },
    })
    await user.click(within(form).getByRole("button", { name: "Send" }))
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0]).toEqual({
      path: "/api/v1/threads",
      body: {
        target: "dashboard:dashboard",
        body: "Raise the threshold",
        version: 3,
        panel_id: "latency",
      },
    })
  })

  it("puts the cursor back in the field after a send when focus stayed in the form", async () => {
    const user = userEvent.setup()
    const { form } = await renderComposer()
    await user.click(within(form).getByRole("button", { name: "Send" }))
    expect(await within(form).findByText("Comment added")).toBeInTheDocument()
    expect(
      within(form).getByRole("textbox", { name: "Comment or request" })
    ).toHaveFocus()
  })

  it("leaves focus alone when the person moved it elsewhere while the send was in flight", async () => {
    let release = () => {}
    const posts = stubServer(
      {},
      new Promise<void>((resolve) => {
        release = resolve
      })
    )
    const user = userEvent.setup()
    const { form } = await renderComposer()
    const elsewhere = document.body.appendChild(
      document.createElement("button")
    )
    await user.click(within(form).getByRole("button", { name: "Send" }))
    await waitFor(() => expect(posts).toHaveLength(1))
    elsewhere.focus()
    release()
    expect(await within(form).findByText("Comment added")).toBeInTheDocument()
    expect(elsewhere).toHaveFocus()
    elsewhere.remove()
  })

  it("sends start_runs: true when the comment mentions @observe and shows the run line", async () => {
    const posts = stubServer({
      run: {
        id: "6f1e2d3c-4b5a-4968-8776-655443322110",
        status: "queued",
        assigned_kind: "builtin",
      },
    })
    const user = userEvent.setup()
    const { form } = await renderComposer({
      draft: "@observe raise the threshold",
    })
    await user.click(
      within(form).getByRole("button", { name: "Send and ask Observe Agent" })
    )
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].path).toBe("/api/v1/threads")
    expect(posts[0].body).toMatchObject({
      target: "dashboard:dashboard",
      body: "@observe raise the threshold",
      version: 3,
      start_runs: true,
    })
    expect(
      await within(form).findByText(/Observe Agent is working on it/)
    ).toBeVisible()
  })

  it("offers the person's connected agents in the mention picker", async () => {
    const user = userEvent.setup()
    const { form } = await renderComposer()
    await user.click(
      within(form).getByRole("combobox", {
        name: "Mention a person or an agent",
      })
    )
    expect(
      await screen.findByRole("option", { name: /Claude Code/ })
    ).toBeVisible()
    expect(screen.getByRole("option", { name: /Observe Agent/ })).toBeVisible()
  })

  it("blocks sending a message that mentions two agents", async () => {
    const { form } = await renderComposer({
      draft: `@observe @[Claude Code](grant:${grantId}) go`,
    })
    expect(
      within(form).getByText("Mention one agent per message")
    ).toBeVisible()
    expect(within(form).getByRole("button", { name: "Send" })).toBeDisabled()
  })

  it("keeps a plain comment sendable and says why when the agent is blocked", async () => {
    const reason = "Finish editing the layout to send changes to the agent."
    const blocked: AgentGate = { kind: "blocked", reason }
    const { form } = await renderComposer({ gate: blocked })
    expect(within(form).getByRole("button", { name: "Send" })).toBeEnabled()
    expect(within(form).getByText(reason)).toBeVisible()
  })

  it("holds an agent mention back while the agent is blocked", async () => {
    const blocked: AgentGate = { kind: "blocked", reason: "Wait." }
    const { form } = await renderComposer({
      gate: blocked,
      draft: "@observe go",
    })
    expect(
      within(form).getByRole("button", { name: "Send and ask Observe Agent" })
    ).toBeDisabled()
  })

  it.each([
    [true, "Investigate"],
    [false, "Send"],
  ])(
    "with Edit %s, typing /investigate makes the send button say %s",
    async (canEdit, label) => {
      const user = userEvent.setup()
      function Harness() {
        const [draft, setDraft] = useState("")
        return (
          <QueryClientProvider client={testQueryClient()}>
            <ActivityComposer
              dashboardId="dashboard"
              latestVersion={3}
              panels={panels}
              target={{ kind: "dashboard" }}
              onTargetChange={vi.fn()}
              draft={draft}
              onDraftChange={setDraft}
              gate={{ kind: "allowed" }}
              canEdit={canEdit}
              placeholder="Write a comment"
              range={{ from: "now-1h", to: "now" }}
            />
          </QueryClientProvider>
        )
      }
      render(<Harness />)
      await user.type(
        screen.getByRole("textbox", { name: "Comment or request" }),
        "/investigate why is latency up"
      )
      expect(screen.getByRole("button", { name: label })).toBeVisible()
      expect(
        screen.getByRole("textbox", { name: "Comment or request" })
      ).toHaveValue(
        canEdit ? "why is latency up" : "/investigate why is latency up"
      )
    }
  )

  it("disables sending while the draft is empty", async () => {
    const { form } = await renderComposer({ draft: "  " })
    expect(within(form).getByRole("button", { name: "Send" })).toBeDisabled()
  })
})
