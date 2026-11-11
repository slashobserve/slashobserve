import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { Message, Thread, ThreadPage } from "@/comments/thread-queries"
import { renderWithProviders } from "@/test/render"

import { FindingComments } from "./finding-comments"

const findingId = "6b1f8a52-0c1d-4e7a-9a43-2d5e8f0a1b3c"
const ada = {
  kind: "user" as const,
  id: "u-ada",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function thread(overrides: Partial<Thread> = {}): Thread {
  return {
    id: "t1",
    target: { kind: "finding", id: findingId },
    dashboard_id: null,
    version: null,
    panel_id: null,
    panel_present: true,
    status: "open",
    allowed_actions: ["reply", "resolve"],
    created_at: "2026-11-06T10:00:00Z",
    updated_at: "2026-11-06T10:00:00Z",
    resolved_at: null,
    resolved_actor: null,
    resolved_by_version: null,
    revision: null,
    ...overrides,
  }
}

function message(overrides: Partial<Message> = {}): Message {
  return {
    id: "t1",
    thread_id: "t1",
    body: "This matches the deploy",
    actor: ada,
    created_at: "2026-11-06T10:00:00Z",
    updated_at: "2026-11-06T10:00:00Z",
    edited_at: null,
    mentions: [],
    can_edit: false,
    ...overrides,
  }
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function stubServer(
  page: Pick<ThreadPage, "threads" | "messages" | "can_comment">
) {
  const sent: { method: string; path: string; body: unknown }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (request.method === "GET") {
        return json({ ...page, next_cursor: null })
      }
      const text = await request.text()
      sent.push({
        method: request.method,
        path: url.pathname,
        body: text === "" ? null : JSON.parse(text),
      })
      return json({ thread: thread(), message: message() }, 201)
    })
  )
  return sent
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("FindingComments", () => {
  it("says there are no comments and offers a composer when the reader may comment", async () => {
    stubServer({ threads: [], messages: [], can_comment: true })
    renderWithProviders(<FindingComments findingId={findingId} />)
    expect(await screen.findByText("No comments yet.")).toBeInTheDocument()
    expect(
      screen.getByRole("textbox", { name: "Comment on this finding" })
    ).toBeInTheDocument()
  })

  it("lists each thread with its messages, actors and status", async () => {
    stubServer({
      threads: [thread()],
      messages: [
        message(),
        message({
          id: "m2",
          body: "Rolled back",
          created_at: "2026-11-06T10:05:00Z",
          actor: { ...ada, id: "u-grace", name: "Grace", label: "Grace" },
        }),
      ],
      can_comment: true,
    })
    renderWithProviders(<FindingComments findingId={findingId} />)
    const list = await screen.findByRole("list", { name: "Comment thread" })
    expect(
      within(list).getByText("This matches the deploy")
    ).toBeInTheDocument()
    expect(within(list).getByText("Rolled back")).toBeInTheDocument()
    expect(screen.getByText("Open")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Resolve" })).toBeInTheDocument()
  })

  it("shows a resolved thread with who resolved it and Reopen", async () => {
    stubServer({
      threads: [
        thread({
          status: "resolved",
          allowed_actions: ["reply", "reopen"],
          resolved_at: "2026-11-06T12:00:00Z",
          resolved_actor: { ...ada, label: "Grace" },
        }),
      ],
      messages: [message()],
      can_comment: true,
    })
    renderWithProviders(<FindingComments findingId={findingId} />)
    expect(await screen.findByText("Resolved by Grace")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Reopen" })).toBeInTheDocument()
  })

  it("hides the composer and every action when the reader cannot comment", async () => {
    stubServer({
      threads: [thread({ allowed_actions: [] })],
      messages: [message({ can_edit: false })],
      can_comment: false,
    })
    renderWithProviders(<FindingComments findingId={findingId} />)
    expect(
      await screen.findByText("This matches the deploy")
    ).toBeInTheDocument()
    expect(screen.queryByRole("textbox")).toBeNull()
    expect(screen.queryByRole("button", { name: "Reply" })).toBeNull()
    expect(screen.queryByRole("button", { name: "Resolve" })).toBeNull()
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull()
  })

  it("offers only the actions the server lists for the thread", async () => {
    stubServer({
      threads: [thread({ allowed_actions: ["reply"] })],
      messages: [message()],
      can_comment: true,
    })
    renderWithProviders(<FindingComments findingId={findingId} />)
    expect(
      await screen.findByRole("button", { name: "Reply" })
    ).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Resolve" })).toBeNull()
    expect(screen.queryByRole("button", { name: "Reopen" })).toBeNull()
  })

  it("starts a thread on the finding from the composer", async () => {
    const sent = stubServer({ threads: [], messages: [], can_comment: true })
    const { user } = renderWithProviders(
      <FindingComments findingId={findingId} />
    )
    const field = await screen.findByRole("textbox", {
      name: "Comment on this finding",
    })
    await user.type(field, "Looks like the deploy")
    await user.click(screen.getByRole("button", { name: "Comment" }))
    await waitFor(() => expect(sent).toHaveLength(1))
    expect(sent[0]).toEqual({
      method: "POST",
      path: "/api/v1/threads",
      body: { target: `finding:${findingId}`, body: "Looks like the deploy" },
    })
    await waitFor(() => expect(field).toHaveValue(""))
  })

  it("replies to a thread and resolves it", async () => {
    const sent = stubServer({
      threads: [thread()],
      messages: [message()],
      can_comment: true,
    })
    const { user } = renderWithProviders(
      <FindingComments findingId={findingId} />
    )
    await user.click(await screen.findByRole("button", { name: "Reply" }))
    await user.type(screen.getByRole("textbox", { name: "Reply" }), "Agreed")
    await user.click(
      within(screen.getByRole("form", { name: "Reply" })).getByRole("button", {
        name: "Reply",
      })
    )
    await waitFor(() => expect(sent).toHaveLength(1))
    expect(sent[0]).toEqual({
      method: "POST",
      path: "/api/v1/threads/t1/messages",
      body: { body: "Agreed" },
    })
    await user.click(await screen.findByRole("button", { name: "Resolve" }))
    await waitFor(() => expect(sent).toHaveLength(2))
    expect(sent[1].path).toBe("/api/v1/threads/t1/resolve")
  })

  it("offers a retry when the comments cannot be loaded", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => json({ error: "boom", code: "internal" }, 500))
    )
    renderWithProviders(<FindingComments findingId={findingId} />)
    expect(
      await screen.findByText("Could not load the comments")
    ).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument()
  })
})
