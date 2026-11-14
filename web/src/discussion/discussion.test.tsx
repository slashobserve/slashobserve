import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Message, Thread } from "@/comments/thread-queries"
import { renderWithProviders } from "@/test/render"

import { Discussion } from "./discussion"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "viewer-1" } }),
}))

const target = "observation:3f2c8d1e-0b5a-4c7d-9e11-2a6b8c4d5e70"

function actor(id: string, name: string) {
  return {
    kind: "user" as const,
    id,
    name,
    origin: "web" as const,
    client: null,
    run_id: null,
    label: name,
  }
}

const ada = actor("viewer-1", "Ada")
const grace = actor("grace-1", "Grace")

function message(
  id: string,
  who: ReturnType<typeof actor>,
  createdAt: string
): Message {
  return {
    id,
    thread_id: "t1",
    body: `body of ${id}`,
    actor: who,
    created_at: createdAt,
    updated_at: createdAt,
    edited_at: null,
    mentions: [],
    can_edit: false,
  }
}

const thread: Thread = {
  id: "t1",
  target: { kind: "observation", id: "f1" },
  dashboard_id: null,
  version: null,
  panel_id: null,
  panel_present: true,
  status: "open",
  allowed_actions: ["reply", "resolve"],
  created_at: "2020-11-10T09:00:00Z",
  updated_at: "2020-11-10T09:05:00Z",
  resolved_at: null,
  resolved_actor: null,
  resolved_by_version: null,
  revision: null,
}

const earlier = message("m1", grace, "2020-11-10T09:00:00Z")
const later = message("m2", ada, "2020-11-10T09:05:00Z")

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

type Server = {
  threads: Thread[]
  messages: () => Message[]
  canComment?: boolean
  sent?: Partial<Message>
}

function mockServer({
  threads,
  messages,
  canComment = true,
  sent = {},
}: Server) {
  const posts: { path: string; body: unknown }[] = []
  const fetch = vi.fn(async (request: Request) => {
    const { pathname } = new URL(request.url)
    if (request.method === "POST") {
      posts.push({ path: pathname, body: await request.clone().json() })
      return json({ thread, message: { ...later, ...sent } }, 201)
    }
    if (pathname === "/api/v1/agent") {
      return json({
        available: true,
        provider: "anthropic",
        model: "claude-haiku-5-5",
        effort: null,
      })
    }
    if (pathname === "/api/v1/agents") return json({ service_accounts: [] })
    return json({
      threads,
      messages: messages(),
      next_cursor: null,
      can_comment: canComment,
    })
  })
  vi.stubGlobal("fetch", fetch)
  return posts
}

afterEach(() => {
  vi.unstubAllGlobals()
})

function renderDiscussion(
  props: Partial<Parameters<typeof Discussion>[0]> & {
    route?: { path: string; url: string }
  } = {}
) {
  const { route, ...discussion } = props
  return renderWithProviders(
    <Discussion
      target={target}
      hint="Posted to everyone who can see this"
      emptyText="No messages yet."
      {...discussion}
    />,
    { route }
  )
}

describe("Discussion", () => {
  it("lists every message in time order with its author label", async () => {
    mockServer({ threads: [thread], messages: () => [later, earlier] })
    renderDiscussion()
    const list = await screen.findByRole("list", { name: "Discussion" })
    const rows = within(list)
      .getAllByRole("listitem")
      .filter((row) => !row.hasAttribute("data-day"))
    expect(rows).toHaveLength(2)
    expect(rows[0]).toHaveTextContent("Grace")
    expect(rows[0]).toHaveTextContent("body of m1")
    expect(rows[1]).toHaveTextContent("Ada")
    expect(rows[1]).toHaveTextContent("body of m2")
  })

  it("replies in the thread with the typed body", async () => {
    const posts = mockServer({
      threads: [thread],
      messages: () => [earlier, later],
    })
    const { user } = renderDiscussion()
    await screen.findByRole("list", { name: "Discussion" })
    await user.type(
      screen.getByRole("textbox", { name: "Message" }),
      "Rolling back now"
    )
    await user.click(screen.getByRole("button", { name: "Send" }))
    await waitFor(() =>
      expect(posts).toEqual([
        {
          path: "/api/v1/threads/t1/messages",
          body: { body: "Rolling back now" },
        },
      ])
    )
  })

  it("opens a thread on the target when there is none", async () => {
    const posts = mockServer({ threads: [], messages: () => [] })
    const { user } = renderDiscussion()
    expect(await screen.findByText("No messages yet.")).toBeInTheDocument()
    await user.type(screen.getByRole("textbox", { name: "Message" }), "First")
    await user.click(screen.getByRole("button", { name: "Send" }))
    await waitFor(() =>
      expect(posts).toEqual([
        { path: "/api/v1/threads", body: { target, body: "First" } },
      ])
    )
  })

  it("tells a reader who cannot comment why the composer is off", async () => {
    mockServer({ threads: [], messages: () => [], canComment: false })
    renderDiscussion()
    await screen.findByText("No messages yet.")
    expect(
      screen.getByText(
        "You can read this discussion. People who can comment write here."
      )
    ).toBeInTheDocument()
    expect(screen.getByRole("textbox", { name: "Message" })).toBeDisabled()
  })

  it("renders the message actions once per message", async () => {
    mockServer({ threads: [thread], messages: () => [earlier, later] })
    renderDiscussion({
      messageActions: (m) => <button type="button">Act on {m.id}</button>,
    })
    await screen.findByRole("list", { name: "Discussion" })
    expect(screen.getAllByRole("button", { name: /^Act on / })).toHaveLength(2)
    expect(
      screen.getByRole("button", { name: "Act on m1" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Act on m2" })
    ).toBeInTheDocument()
  })

  it("announces another person's new message but not the first load or one's own", async () => {
    const newer = message("m3", grace, "2020-11-10T09:10:00Z")
    const own = message("m4", ada, "2020-11-10T09:11:00Z")
    let sent = false
    const posts = mockServer({
      threads: [thread],
      messages: () => (sent ? [earlier, later, newer, own] : [earlier, later]),
    })
    const { user } = renderDiscussion()
    await screen.findByRole("list", { name: "Discussion" })
    const status = screen.getByRole("status")
    expect(status).toHaveTextContent("")

    sent = true
    await user.type(screen.getByRole("textbox", { name: "Message" }), "Done")
    await user.click(screen.getByRole("button", { name: "Send" }))
    await waitFor(() => expect(posts).toHaveLength(1))
    await waitFor(() =>
      expect(status).toHaveTextContent(/^New message from Grace\s*$/)
    )
  })

  it("sends start_runs when the message mentions one agent and shows the working line with a link to the run", async () => {
    const runId = "6f1e2d3c-4b5a-4968-8776-655443322110"
    const posts = mockServer({
      threads: [thread],
      messages: () => [earlier],
      sent: { run: { id: runId, status: "queued", assigned_kind: "builtin" } },
    })
    const { user } = renderDiscussion({
      route: { path: "/", url: "/" },
    })
    await screen.findByRole("list", { name: "Discussion" })
    await user.click(screen.getByRole("textbox", { name: "Message" }))
    await user.paste("@observe why did latency rise")
    await user.click(
      screen.getByRole("button", { name: "Send and ask Observe Agent" })
    )
    await waitFor(() =>
      expect(posts).toEqual([
        {
          path: "/api/v1/threads/t1/messages",
          body: { body: "@observe why did latency rise", start_runs: true },
        },
      ])
    )
    expect(
      await screen.findByText(/Observe Agent is working on it/)
    ).toBeVisible()
    expect(screen.getByRole("link", { name: "View the run" })).toHaveAttribute(
      "href",
      `/c/${runId}`
    )
  })

  it("shows the refusal sentence when the server stored the message and started no run", async () => {
    mockServer({
      threads: [thread],
      messages: () => [earlier],
      sent: {
        run_refusal: {
          code: "agent_unavailable",
          message: "the agent is not configured",
        },
      },
    })
    const { user } = renderDiscussion()
    await screen.findByRole("list", { name: "Discussion" })
    await user.click(screen.getByRole("textbox", { name: "Message" }))
    await user.paste("@observe hi")
    await user.click(
      screen.getByRole("button", { name: "Send and ask Observe Agent" })
    )
    expect(
      await screen.findByText("The agent is not configured.")
    ).toBeVisible()
  })

  it("sends no start_runs for a message that mentions only a person", async () => {
    const posts = mockServer({ threads: [thread], messages: () => [earlier] })
    const { user } = renderDiscussion()
    await screen.findByRole("list", { name: "Discussion" })
    await user.click(screen.getByRole("textbox", { name: "Message" }))
    await user.paste("@[Grace](user:2f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f) look")
    await user.click(screen.getByRole("button", { name: "Send" }))
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].body).toEqual({
      body: "@[Grace](user:2f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f) look",
    })
  })
})
