import { screen, waitFor, within } from "@testing-library/react"
import { useState } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { pinnedText, ungated, type GateInput } from "@/conversation/delivery"
import { renderWithProviders } from "@/test/render"

import {
  ActivityComposer,
  type Replying,
  type Target,
} from "./activity-composer"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "editor" } }),
}))

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

type Post = { path: string; body: Record<string, unknown> }

function stubServer(
  sent: Record<string, unknown> = {},
  postHeld: () => Promise<void> = () => Promise.resolve()
) {
  const posts: Post[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url)
      if (request.method === "POST") {
        posts.push({ path: pathname, body: await request.clone().json() })
        await postHeld()
        return json(
          {
            thread: { id: "t1" },
            message: { id: "m1", thread_id: "t1", body: "", ...sent },
            to: "everyone",
            replayed: false,
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

const panels = [
  { id: "latency", title: "Latency" },
  { id: "errors", title: "Errors" },
]

function renderComposer({
  gate = ungated,
  draft: initialDraft = "Raise the threshold",
  target = { kind: "dashboard" },
  replying: initialReplying = null,
}: {
  gate?: GateInput
  draft?: string
  target?: Target
  replying?: Replying | null
} = {}) {
  const onTargetChange = vi.fn<(target: Target) => void>()
  function Harness() {
    const [draft, setDraft] = useState(initialDraft)
    const [replying, setReplying] = useState(initialReplying)
    return (
      <ActivityComposer
        dashboardId="dashboard"
        panels={panels}
        target={target}
        onTargetChange={onTargetChange}
        replying={replying}
        onLeaveThread={() => setReplying(null)}
        draft={draft}
        onDraftChange={setDraft}
        gate={gate}
        placeholder="Write a comment"
      />
    )
  }
  const result = renderWithProviders(<Harness />, {
    route: { path: "/", url: "/" },
  })
  return { ...result, onTargetChange }
}

async function form() {
  return screen.findByRole("form", { name: "Add to the activity" })
}

async function field() {
  return within(await form()).findByRole("textbox", {
    name: "Comment or request",
  })
}

const thread: Replying = {
  threadId: "t9",
  label: "Latency",
  agent: null,
  agentOff: false,
  people: [],
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("ActivityComposer", () => {
  beforeEach(() => {
    stubServer()
  })

  it("targets a panel through the On select", async () => {
    const { user, onTargetChange } = renderComposer()
    await user.click(within(await form()).getByRole("combobox", { name: "On" }))
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual([
      "The whole dashboard",
      "Latency",
      "Errors",
    ])
    await user.click(screen.getByRole("option", { name: "Errors" }))
    expect(onTargetChange).toHaveBeenCalledWith({
      kind: "panel",
      panelId: "errors",
    })
  })

  it("posts a plain comment on a panel with a client id and leaves the recipient to the server", async () => {
    const posts = stubServer()
    const { user } = renderComposer({
      target: { kind: "panel", panelId: "latency" },
    })
    expect(within(await form()).getByText("To: everyone here")).toBeVisible()
    await user.click(within(await form()).getByRole("button", { name: "Send" }))
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0]).toEqual({
      path: "/api/v1/threads",
      body: {
        target: "dashboard:dashboard",
        body: "Raise the threshold",
        client_id: expect.any(String),
        panel_id: "latency",
      },
    })
  })

  it("sends on Enter and adds a line on Shift+Enter", async () => {
    const posts = stubServer()
    const { user } = renderComposer({ draft: "" })
    await user.type(await field(), "one{Shift>}{Enter}{/Shift}two")
    expect(posts).toHaveLength(0)
    await user.keyboard("{Enter}")
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].body.body).toBe("one\ntwo")
    expect(await field()).toHaveValue("")
  })

  it("sends to the agent when the To chip is turned, with the agent's name on the button", async () => {
    const posts = stubServer()
    const { user } = renderComposer()
    await user.click(
      within(await form()).getByRole("button", {
        name: /To: everyone here/,
      })
    )
    expect(within(await form()).getByText("To: Observe Agent")).toBeVisible()
    await user.click(
      within(await form()).getByRole("button", {
        name: "Send and ask Observe Agent",
      })
    )
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].body).toMatchObject({ to: "agent" })
  })

  it("keeps the thread's agent on when a person is mentioned in a reply", async () => {
    const posts = stubServer()
    const { user } = renderComposer({
      draft:
        "@[Alice](user:0b7e6a51-2c3d-4e5f-8a9b-0c1d2e3f4a5b) can you look?",
      replying: { ...thread, agent: { kind: "builtin", id: null } },
    })
    expect(within(await form()).getByText("To: everyone here")).toBeVisible()
    await user.click(within(await form()).getByRole("button", { name: "Send" }))
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].path).toBe("/api/v1/threads/t9/messages")
    expect(posts[0].body).not.toHaveProperty("to")
  })

  it("asks the mentioned agent and shows the run line", async () => {
    const posts = stubServer({
      run: {
        id: "6f1e2d3c-4b5a-4968-8776-655443322110",
        status: "queued",
        assigned_kind: "builtin",
      },
    })
    const { user } = renderComposer({ draft: "@observe raise the threshold" })
    await user.click(
      within(await form()).getByRole("button", {
        name: "Send and ask Observe Agent",
      })
    )
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].body).toMatchObject({
      body: "@observe raise the threshold",
    })
    expect(posts[0].body).not.toHaveProperty("to")
    expect(
      await within(await form()).findByText(/Observe Agent is working on it/)
    ).toBeVisible()
  })

  it("holds an agent send back on an older version but keeps a comment sendable", async () => {
    const gate = { ...ungated, pinned: true }
    const { unmount } = renderComposer({ gate, draft: "@observe go" })
    expect(within(await form()).getByText(pinnedText)).toBeVisible()
    expect(
      within(await form()).getByRole("button", {
        name: "Send and ask Observe Agent",
      })
    ).toHaveAttribute("aria-disabled", "true")
    unmount()
    renderComposer({ gate, draft: "a note" })
    expect(
      within(await form()).getByRole("button", { name: "Send" })
    ).toBeEnabled()
    expect(screen.queryByText(pinnedText)).toBeNull()
  })

  it("blocks a message that mentions two agents", async () => {
    renderComposer({ draft: `@observe @[Claude Code](grant:${grantId}) go` })
    expect(
      within(await form()).getByText("Mention one agent per message")
    ).toBeVisible()
  })

  it("opens a thread and sends two replies without another click", async () => {
    const posts = stubServer()
    const { user } = renderComposer({ draft: "", replying: thread })
    expect(within(await form()).getByText("Replying in Latency")).toBeVisible()
    expect(
      within(await form()).queryByRole("combobox", { name: "On" })
    ).toBeNull()
    await user.type(await field(), "first{Enter}")
    await waitFor(() => expect(posts).toHaveLength(1))
    await waitFor(async () => expect(await field()).toHaveValue(""))
    await user.type(await field(), "second{Enter}")
    await waitFor(() => expect(posts).toHaveLength(2))
    expect(posts.map((post) => [post.path, post.body.body])).toEqual([
      ["/api/v1/threads/t9/messages", "first"],
      ["/api/v1/threads/t9/messages", "second"],
    ])
    expect(within(await form()).getByText("Replying in Latency")).toBeVisible()
  })

  it("returns to a new comment when the thread chip is removed", async () => {
    const posts = stubServer()
    const { user } = renderComposer({ replying: thread })
    await user.click(
      within(await form()).getByRole("button", {
        name: "Remove Replying in Latency",
      })
    )
    expect(within(await form()).queryByText("Replying in Latency")).toBeNull()
    expect(await field()).toHaveFocus()
    await user.keyboard("{Enter}")
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].path).toBe("/api/v1/threads")
  })

  it("delivers a double send once", async () => {
    let release = () => {}
    const held = new Promise<void>((resolve) => {
      release = resolve
    })
    const posts = stubServer({}, () => held)
    const { user } = renderComposer()
    await user.click(await field())
    await user.keyboard("{Enter}{Enter}")
    await user.click(within(await form()).getByRole("button", { name: "Send" }))
    release()
    await waitFor(async () => expect(await field()).toHaveValue(""))
    expect(posts).toHaveLength(1)
  })
})
