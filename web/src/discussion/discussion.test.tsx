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
  target: { kind: "finding", id: "f1" },
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
}

function mockServer({ threads, messages, canComment = true }: Server) {
  const posts: { path: string; body: unknown }[] = []
  const fetch = vi.fn(async (request: Request) => {
    const { pathname } = new URL(request.url)
    if (request.method === "POST") {
      posts.push({ path: pathname, body: await request.clone().json() })
      return json({ thread, message: later }, 201)
    }
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
  props: Partial<Parameters<typeof Discussion>[0]> = {}
) {
  return renderWithProviders(
    <Discussion
      target={target}
      hint="Posted to everyone who can see this"
      emptyText="No messages yet."
      {...props}
    />
  )
}

describe("Discussion", () => {
  it("lists every message in time order with its author label", async () => {
    mockServer({ threads: [thread], messages: () => [later, earlier] })
    renderDiscussion()
    const list = await screen.findByRole("list", { name: "Messages" })
    const rows = within(list).getAllByRole("listitem")
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
    await screen.findByRole("list", { name: "Messages" })
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
    await screen.findByRole("list", { name: "Messages" })
    expect(screen.getAllByRole("button", { name: /^Act on / })).toHaveLength(2)
    expect(
      screen.getByRole("button", { name: "Act on m1" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Act on m2" })
    ).toBeInTheDocument()
  })

  it("offers Hide only when a handler is given", async () => {
    mockServer({ threads: [thread], messages: () => [earlier] })
    const onHide = vi.fn()
    const { user } = renderDiscussion({ onHide })
    await user.click(
      await screen.findByRole("button", { name: "Hide discussion" })
    )
    expect(onHide).toHaveBeenCalledOnce()
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
    await screen.findByRole("list", { name: "Messages" })
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
})
