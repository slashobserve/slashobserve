import { fireEvent, screen, waitFor, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Message, Thread } from "@/comments/thread-queries"
import type { Finding } from "@/findings/queries"
import { renderWithProviders } from "@/test/render"

import { Discussion, DiscussionFeed } from "./discussion"
import { must } from "@/test/must"

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
  who: Message["actor"],
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
    can_delete: false,
    block_id: null,
    reply_to: null,
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
  unread: false,
  created_at: "2020-11-10T09:00:00Z",
  updated_at: "2020-11-10T09:05:00Z",
  resolved_at: null,
  resolved_actor: null,
  resolved_by_version: null,
  revision: null,
  anchor: null,
  block_id: null,
  agent: null,
  agent_off: false,
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
  observe?: (method: string) => Record<string, unknown>
}

function mockServer({
  threads,
  messages,
  canComment = true,
  sent = {},
  observe,
}: Server) {
  const posts: { path: string; body: unknown }[] = []
  const fetch = vi.fn(async (request: Request) => {
    const { pathname } = new URL(request.url)
    if (observe !== undefined && pathname.startsWith("/api/v1/observe/")) {
      return json({ request: observe(request.method) })
    }
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

  it("marks the opening message of the focused thread and no other", async () => {
    const opening = message("t1", grace, "2020-11-10T09:00:00Z")
    mockServer({ threads: [thread], messages: () => [opening, later] })
    renderDiscussion({ focusedThreadId: "t1" })
    const list = await screen.findByRole("list", { name: "Discussion" })
    const rows = within(list)
      .getAllByRole("listitem")
      .filter((row) => !row.hasAttribute("data-day"))
    expect(rows[0]).toHaveAttribute("aria-current", "true")
    expect(rows[1]).not.toHaveAttribute("aria-current")
  })

  it("marks nothing when no thread is focused", async () => {
    const opening = message("t1", grace, "2020-11-10T09:00:00Z")
    mockServer({ threads: [thread], messages: () => [opening, later] })
    renderDiscussion()
    const list = await screen.findByRole("list", { name: "Discussion" })
    expect(list.querySelector("[aria-current]")).toBeNull()
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
          body: {
            body: "Rolling back now",
            client_id: expect.any(String),
            time_zone: expect.any(String),
          },
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
        {
          path: "/api/v1/threads",
          body: {
            target,
            body: "First",
            client_id: expect.any(String),
            time_zone: expect.any(String),
          },
        },
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
    const status = screen.getAllByRole("status").at(-1)
    expect(status).toHaveTextContent("")

    sent = true
    await user.type(screen.getByRole("textbox", { name: "Message" }), "Done")
    await user.click(screen.getByRole("button", { name: "Send" }))
    await waitFor(() => expect(posts).toHaveLength(1))
    await waitFor(() =>
      expect(status).toHaveTextContent(/^New message from Grace\s*$/)
    )
  })

  it("asks the agent when the message mentions one agent and shows the working line with a link to the run", async () => {
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
          body: {
            body: "@observe why did latency rise",
            client_id: expect.any(String),
            time_zone: expect.any(String),
          },
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

  it("leaves the recipient of a message that mentions only a person to the server", async () => {
    const posts = mockServer({ threads: [thread], messages: () => [earlier] })
    const { user } = renderDiscussion()
    await screen.findByRole("list", { name: "Discussion" })
    await user.click(screen.getByRole("textbox", { name: "Message" }))
    await user.paste("@[Grace](user:2f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f) look")
    await user.click(screen.getByRole("button", { name: "Send" }))
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(must(posts[0]).body).toEqual({
      body: "@[Grace](user:2f1f0d1e-9a3b-4c2d-8e5f-6a7b8c9d0e1f) look",
      client_id: expect.any(String),
      time_zone: expect.any(String),
    })
  })
})

describe("Discussion with a run", () => {
  const runId = "6f1e2d3c-4b5a-4968-8776-655443322110"
  const agent: Message["actor"] = {
    ...grace,
    run_id: runId,
    label: "Observe Agent",
  }
  const asked: Message = {
    ...earlier,
    body: "@observe why did latency rise",
    run: { id: runId, status: "running", assigned_kind: "builtin" },
  }

  function finding(patch: Partial<Finding> = {}): Finding {
    return {
      id: "f1",
      observation_id: null,
      agent_request_id: runId,
      dashboard_id: null,
      folder_id: null,
      ask_on: null,
      restricted: false,
      title: "Latency rose",
      markdown: "Latency **doubled** after the deploy.",
      evidence: [
        { id: "e1", type: "timeseries", title: "Latency" },
      ] as unknown as Finding["evidence"],
      from: "2020-11-10T08:00:00Z",
      to: "2020-11-10T09:00:00Z",
      as_of: null,
      annotation_id: null,
      created_by: { kind: "user", id: "grace-1", name: "Grace" },
      actor: agent,
      created_at: "2020-11-10T09:01:00Z",
      url: null,
      suggested_change: null,
      observe_worthy: false,
      ...patch,
    }
  }

  function answered(canSave = true): Message {
    return {
      ...message("m2", agent, "2020-11-10T09:01:00Z"),
      body: "Answered.",
      answer: { finding: finding(), can_save: canSave },
    }
  }

  it("shows the answer card without the stub reply body", async () => {
    mockServer({ threads: [thread], messages: () => [asked, answered()] })
    renderDiscussion()
    const list = await screen.findByRole("list", { name: "Discussion" })
    expect(await within(list).findByText("doubled")).toBeInTheDocument()
    expect(within(list).queryByText("Answered.")).not.toBeInTheDocument()
    expect(
      within(list).getByRole("button", { name: "1 evidence panel" })
    ).toBeInTheDocument()
  })

  it("keeps a reply body that is not the stub text beside the answer card", async () => {
    const restricted: Message = {
      ...answered(),
      body: "The answer is kept where only Admins read it.",
    }
    mockServer({ threads: [thread], messages: () => [asked, restricted] })
    renderDiscussion()
    const list = await screen.findByRole("list", { name: "Discussion" })
    expect(await within(list).findByText(restricted.body)).toBeInTheDocument()
    expect(await within(list).findByText("doubled")).toBeInTheDocument()
  })

  it("keeps the stub reply body when the reader cannot see the answer", async () => {
    const hidden: Message = { ...answered(), answer: null }
    mockServer({ threads: [thread], messages: () => [asked, hidden] })
    renderDiscussion()
    const list = await screen.findByRole("list", { name: "Discussion" })
    expect(await within(list).findByText("Answered.")).toBeInTheDocument()
    expect(within(list).queryByText("doubled")).not.toBeInTheDocument()
  })

  function saveableAnswers(): Message[] {
    return [
      asked,
      {
        ...answered(),
        answer: {
          finding: finding({ observe_worthy: true }),
          can_save: true,
        },
      },
    ]
  }

  it("does not offer Create observation inside an observation's discussion", async () => {
    mockServer({ threads: [thread], messages: saveableAnswers })
    renderDiscussion()
    expect(
      await screen.findByRole("button", { name: "1 evidence panel" })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Create observation" })
    ).not.toBeInTheDocument()
  })

  it("never offers to revise the dashboard from an observation", async () => {
    mockServer({
      threads: [thread],
      messages: () => [
        asked,
        {
          ...answered(),
          answer: {
            finding: finding({ suggested_change: "Add a p99 panel" }),
            can_save: true,
          },
        },
      ],
    })
    renderDiscussion()
    expect(
      await screen.findByText("Suggested change: Add a p99 panel")
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Revise the dashboard" })
    ).not.toBeInTheDocument()
  })

  it("shows the working line under a message whose run has not answered", async () => {
    mockServer({ threads: [thread], messages: () => [asked] })
    renderDiscussion({ route: { path: "/", url: "/" } })
    const list = await screen.findByRole("list", { name: "Discussion" })
    expect(
      within(list).getByText(/Observe Agent is working on it/)
    ).toBeVisible()
    expect(
      within(list).getByRole("link", { name: "View the run" })
    ).toHaveAttribute("href", `/c/${runId}`)
  })

  it("offers Stop under the working line and shows Stopped. after it", async () => {
    let canceled = false
    mockServer({
      threads: [thread],
      messages: () => [asked],
      observe: (method) => {
        if (method === "POST") canceled = true
        return {
          id: runId,
          status: canceled ? "failed" : "running",
          state: canceled ? "canceled" : "running",
          can_cancel: !canceled,
        }
      },
    })
    renderDiscussion({ route: { path: "/", url: "/" } })
    const list = await screen.findByRole("list", { name: "Discussion" })
    fireEvent.click(await within(list).findByRole("button", { name: "Stop" }))
    await waitFor(() => expect(within(list).getByText(/Stopped/)).toBeVisible())
    expect(within(list).queryByRole("button", { name: "Stop" })).toBeNull()
  })

  it("shows one working line while the sent run is in flight", async () => {
    let listed: Message[] = [earlier]
    const posts = mockServer({
      threads: [thread],
      messages: () => listed,
      sent: { run: must(asked.run) },
    })
    const { user } = renderDiscussion({ route: { path: "/", url: "/" } })
    await screen.findByRole("list", { name: "Discussion" })
    await user.click(screen.getByRole("textbox", { name: "Message" }))
    await user.paste("@observe why did latency rise")
    listed = [earlier, asked]
    await user.click(
      screen.getByRole("button", { name: "Send and ask Observe Agent" })
    )
    await waitFor(() => expect(posts).toHaveLength(1))
    await waitFor(() =>
      expect(
        screen.getAllByText(/Observe Agent is working on it/)
      ).toHaveLength(1)
    )
    expect(screen.getAllByRole("link", { name: "View the run" })).toHaveLength(
      1
    )
  })

  it("keeps the composer's working line when the filter hides the message rows", async () => {
    let listed: Message[] = [earlier]
    const posts = mockServer({
      threads: [thread],
      messages: () => listed,
      sent: { run: must(asked.run) },
    })
    const { user } = renderWithProviders(
      <DiscussionFeed
        target={target}
        hint="Posted to everyone who can see this"
        emptyText="No messages yet."
        view={{
          filter: { show: "changes", blockId: null },
          onClear: vi.fn(),
        }}
      />,
      { route: { path: "/", url: "/" } }
    )
    await screen.findByText("No entries match these filters.")
    await user.click(screen.getByRole("textbox", { name: "Message" }))
    await user.paste("@observe why did latency rise")
    listed = [earlier, asked]
    await user.click(
      screen.getByRole("button", { name: "Send and ask Observe Agent" })
    )
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(
      await screen.findByText(/Observe Agent is working on it/)
    ).toBeVisible()
    expect(screen.getAllByText(/Observe Agent is working on it/)).toHaveLength(
      1
    )
  })

  it("drops the working line once the run answered", async () => {
    mockServer({ threads: [thread], messages: () => [asked, answered()] })
    renderDiscussion({ route: { path: "/", url: "/" } })
    const list = await screen.findByRole("list", { name: "Discussion" })
    expect(await within(list).findByText("doubled")).toBeInTheDocument()
    expect(within(list).queryByText(/is working on it/)).not.toBeInTheDocument()
  })
})

describe("DiscussionFeed on an observation", () => {
  const blocks = [
    { id: "b1", title: "Panel: Latency" },
    { id: "b2", title: "Text" },
  ]

  function renderFeed(onShow = vi.fn(), anchor: string | null = null) {
    const onAnchorChange = vi.fn()
    const rendered = renderWithProviders(
      <DiscussionFeed
        target={target}
        hint="Posted to everyone who can see this"
        emptyText="No messages yet."
        blocks={{ items: blocks, anchor, onAnchorChange, onShow }}
        messageActions={(m, { reply }) =>
          reply === null ? null : (
            <button type="button" onClick={reply}>
              Reply to {m.id}
            </button>
          )
        }
      />
    )
    return { ...rendered, onShow, onAnchorChange }
  }

  it("quotes the message a reply answers and names its author in the verb", async () => {
    const quoting: Message = {
      ...later,
      reply_to: { id: "m1", actor: grace, excerpt: "p95 rose after deploy" },
    }
    mockServer({ threads: [thread], messages: () => [earlier, quoting] })
    renderFeed()
    const list = await screen.findByRole("list", { name: "Discussion" })
    const rows = within(list)
      .getAllByRole("listitem")
      .filter((row) => !row.hasAttribute("data-day"))
    expect(rows[1]).toHaveTextContent("replied to Grace")
    expect(rows[1]).toHaveTextContent("p95 rose after deploy")
  })

  it("shows the block a message is on and shows it on the page", async () => {
    const onBlock: Message = { ...later, block_id: "b1" }
    mockServer({ threads: [thread], messages: () => [earlier, onBlock] })
    const { user, onShow } = renderFeed()
    await screen.findByRole("list", { name: "Discussion" })
    expect(screen.getByText("On Panel: Latency")).toBeInTheDocument()
    await user.click(
      screen.getByRole("button", { name: "Show Panel: Latency" })
    )
    expect(onShow).toHaveBeenCalledWith("b1")
  })

  it("shows nothing extra for a block that no longer exists", async () => {
    const gone: Message = { ...later, block_id: "deleted" }
    mockServer({ threads: [thread], messages: () => [earlier, gone] })
    renderFeed()
    await screen.findByRole("list", { name: "Discussion" })
    expect(
      screen.queryByRole("button", { name: /^Show / })
    ).not.toBeInTheDocument()
  })

  it("sends the reply with the quoted message and drops the quote after sending", async () => {
    const posts = mockServer({
      threads: [thread],
      messages: () => [earlier, later],
    })
    const { user } = renderFeed()
    await screen.findByRole("list", { name: "Discussion" })
    await user.click(screen.getByRole("button", { name: "Reply to m1" }))
    expect(screen.getByText("Replying to Grace")).toBeInTheDocument()
    await user.type(screen.getByRole("textbox", { name: "Message" }), "Yes")
    await user.click(screen.getByRole("button", { name: "Send" }))
    await waitFor(() =>
      expect(posts).toEqual([
        {
          path: "/api/v1/threads/t1/messages",
          body: {
            body: "Yes",
            client_id: expect.any(String),
            time_zone: expect.any(String),
            reply_to_id: "m1",
          },
        },
      ])
    )
    await waitFor(() =>
      expect(screen.queryByText("Replying to Grace")).not.toBeInTheDocument()
    )
  })

  it("stops replying when the quote is removed", async () => {
    const posts = mockServer({ threads: [thread], messages: () => [earlier] })
    const { user } = renderFeed()
    await screen.findByRole("list", { name: "Discussion" })
    await user.click(screen.getByRole("button", { name: "Reply to m1" }))
    await user.click(screen.getByRole("button", { name: "Stop replying" }))
    await user.type(screen.getByRole("textbox", { name: "Message" }), "Hi")
    await user.click(screen.getByRole("button", { name: "Send" }))
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(must(posts[0]).body).toEqual({
      body: "Hi",
      client_id: expect.any(String),
      time_zone: expect.any(String),
    })
  })

  it("anchors a new message to the chosen block", async () => {
    const posts = mockServer({ threads: [], messages: () => [] })
    const { user } = renderFeed(vi.fn(), "b1")
    await screen.findByText("No messages yet.")
    expect(
      screen.getByRole("combobox", { name: "Write on" })
    ).toHaveTextContent("On Panel: Latency")
    await user.type(screen.getByRole("textbox", { name: "Message" }), "Why")
    await user.click(screen.getByRole("button", { name: "Send" }))
    await waitFor(() =>
      expect(posts).toEqual([
        {
          path: "/api/v1/threads",
          body: {
            target,
            body: "Why",
            client_id: expect.any(String),
            time_zone: expect.any(String),
            block_id: "b1",
          },
        },
      ])
    )
  })

  it("writes on the whole page when the anchor block is gone", async () => {
    const posts = mockServer({ threads: [thread], messages: () => [earlier] })
    const { user } = renderFeed(vi.fn(), "deleted")
    await screen.findByRole("list", { name: "Discussion" })
    expect(
      screen.getByRole("combobox", { name: "Write on" })
    ).toHaveTextContent("On the whole page")
    await user.type(screen.getByRole("textbox", { name: "Message" }), "Hi")
    await user.click(screen.getByRole("button", { name: "Send" }))
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(must(posts[0]).body).toEqual({
      body: "Hi",
      client_id: expect.any(String),
      time_zone: expect.any(String),
    })
  })
})
