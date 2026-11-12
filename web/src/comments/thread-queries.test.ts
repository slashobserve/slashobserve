import { QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook } from "@testing-library/react"
import { createElement, type ReactNode } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/api/errors"
import { dashboardKeys, observeKeys, threadKeys } from "@/api/keys"
import { testQueryClient } from "@/test/render"

import {
  askedAgent,
  groupThreadPages,
  useOpenThread,
  useReplyToThread,
  type Message,
  type Thread,
} from "./thread-queries"

const ada = {
  kind: "user" as const,
  id: "u1",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function thread(id: string, createdAt: string): Thread {
  return {
    id,
    target: { kind: "finding", id: "f1" },
    dashboard_id: null,
    version: null,
    panel_id: null,
    panel_present: true,
    status: "open",
    allowed_actions: ["reply", "resolve"],
    created_at: createdAt,
    updated_at: createdAt,
    resolved_at: null,
    resolved_actor: null,
    resolved_by_version: null,
    revision: null,
  }
}

function message(id: string, threadId: string, createdAt: string): Message {
  return {
    id,
    thread_id: threadId,
    body: id,
    actor: ada,
    created_at: createdAt,
    updated_at: createdAt,
    edited_at: null,
    mentions: [],
    can_edit: false,
  }
}

describe("groupThreadPages", () => {
  it("returns nothing for no pages", () => {
    expect(groupThreadPages([])).toEqual([])
  })

  it("joins a thread that continues on the next page and orders by creation", () => {
    const first = {
      threads: [
        thread("t2", "2026-11-06T11:00:00Z"),
        thread("t1", "2026-11-06T10:00:00Z"),
      ],
      messages: [
        message("t1", "t1", "2026-11-06T10:00:00Z"),
        message("m3", "t2", "2026-11-06T11:00:00Z"),
      ],
    }
    const second = {
      threads: [thread("t1", "2026-11-06T10:00:00Z")],
      messages: [message("m2", "t1", "2026-11-06T10:30:00Z")],
    }
    const grouped = groupThreadPages([first, second])
    expect(grouped.map((entry) => entry.thread.id)).toEqual(["t1", "t2"])
    expect(grouped[0].messages.map((item) => item.id)).toEqual(["t1", "m2"])
    expect(grouped[1].messages.map((item) => item.id)).toEqual(["m3"])
  })

  it("keeps one copy of a message that two pages both carry", () => {
    const page = {
      threads: [thread("t1", "2026-11-06T10:00:00Z")],
      messages: [message("t1", "t1", "2026-11-06T10:00:00Z")],
    }
    const grouped = groupThreadPages([page, page])
    expect(grouped[0].messages).toHaveLength(1)
  })
})

const runId = "6f1e2d3c-4b5a-4968-8776-655443322110"

function json(body: unknown, status = 201): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function stubPosts(reply: (request: Request) => Response) {
  const posts: { path: string; body: Record<string, unknown> }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      posts.push({
        path: new URL(request.url).pathname,
        body: await request.clone().json(),
      })
      return reply(request)
    })
  )
  return posts
}

function withClient() {
  const queryClient = testQueryClient()
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children)
  return { queryClient, wrapper }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("useOpenThread", () => {
  const sentBody = (extra: Partial<Message> = {}) => ({
    thread: thread("t1", "2026-11-06T10:00:00Z"),
    message: { ...message("t1", "t1", "2026-11-06T10:00:00Z"), ...extra },
  })

  it("sends start_runs only when a run is asked for", async () => {
    const posts = stubPosts(() => json(sentBody()))
    const { wrapper } = withClient()
    const { result } = renderHook(() => useOpenThread("observation:o1"), {
      wrapper,
    })
    await act(() =>
      result.current.mutateAsync({ body: "one", startRuns: false })
    )
    await act(() =>
      result.current.mutateAsync({ body: "two", startRuns: true })
    )
    expect(posts.map((post) => post.body)).toEqual([
      { target: "observation:o1", body: "one" },
      { target: "observation:o1", body: "two", start_runs: true },
    ])
  })

  it("sends the version and panel of a dashboard thread", async () => {
    const posts = stubPosts(() => json(sentBody()))
    const { wrapper } = withClient()
    const { result } = renderHook(() => useOpenThread("dashboard:d1"), {
      wrapper,
    })
    await act(() =>
      result.current.mutateAsync({
        body: "look",
        startRuns: false,
        version: 3,
        panelId: "latency",
      })
    )
    await act(() =>
      result.current.mutateAsync({
        body: "whole",
        startRuns: false,
        version: 3,
        panelId: null,
      })
    )
    expect(posts.map((post) => post.body)).toEqual([
      { target: "dashboard:d1", body: "look", version: 3, panel_id: "latency" },
      { target: "dashboard:d1", body: "whole", version: 3 },
    ])
  })

  it("resolves with the refusal instead of rejecting when the run is refused", async () => {
    const refusal = {
      code: "forbidden_role" as const,
      message: "you cannot start this run",
    }
    stubPosts(() => json(sentBody({ run_refusal: refusal })))
    const { wrapper } = withClient()
    const { result } = renderHook(() => useOpenThread("dashboard:d1"), {
      wrapper,
    })
    const sent = await act(() =>
      result.current.mutateAsync({ body: "@observe go", startRuns: true })
    )
    expect(sent.run).toBeNull()
    expect(sent.runRefusal).toEqual(refusal)
    expect(askedAgent(sent)).toBe(true)
  })

  it("resolves with the started run and refreshes the runs a dashboard shows", async () => {
    const run = {
      id: runId,
      status: "queued" as const,
      assigned_kind: "builtin" as const,
    }
    stubPosts(() => json(sentBody({ run })))
    const { wrapper, queryClient } = withClient()
    const invalidated = vi.spyOn(queryClient, "invalidateQueries")
    const { result } = renderHook(() => useOpenThread("dashboard:d1"), {
      wrapper,
    })
    const sent = await act(() =>
      result.current.mutateAsync({ body: "@observe go", startRuns: true })
    )
    expect(sent.run).toEqual(run)
    const keys = invalidated.mock.calls.map((call) => call[0]?.queryKey)
    expect(keys).toEqual([
      threadKeys.list("dashboard:d1"),
      dashboardKeys.comments("d1"),
      dashboardKeys.revisions("d1"),
      observeKeys.mineAll(),
    ])
  })

  it("leaves the run lists alone for a message that started nothing", async () => {
    stubPosts(() => json(sentBody()))
    const { wrapper, queryClient } = withClient()
    const invalidated = vi.spyOn(queryClient, "invalidateQueries")
    const { result } = renderHook(() => useOpenThread("observation:o1"), {
      wrapper,
    })
    const sent = await act(() =>
      result.current.mutateAsync({ body: "note", startRuns: false })
    )
    expect(askedAgent(sent)).toBe(false)
    expect(invalidated.mock.calls.map((call) => call[0]?.queryKey)).toEqual([
      threadKeys.list("observation:o1"),
    ])
  })

  it.each([
    ["invalid_mention", "that agent is not yours"],
    ["too_many_agents", "start a run for one agent at a time"],
  ])("rejects with the server's sentence on a 422 %s", async (code, text) => {
    stubPosts(() => json({ error: text, code }, 422))
    const { wrapper } = withClient()
    const { result } = renderHook(() => useOpenThread("dashboard:d1"), {
      wrapper,
    })
    const failure = await act(() =>
      result.current
        .mutateAsync({ body: "@observe", startRuns: true })
        .catch((error: unknown) => error)
    )
    expect(failure).toBeInstanceOf(ApiError)
    expect(failure).toMatchObject({ status: 422, code, message: text })
  })
})

describe("useReplyToThread", () => {
  it("sends start_runs only when asked and resolves with the run", async () => {
    const run = { id: runId, status: "queued", assigned_kind: "grant" }
    const posts = stubPosts(() =>
      json({
        thread: thread("t1", "2026-11-06T10:00:00Z"),
        message: { ...message("m2", "t1", "2026-11-06T10:05:00Z"), run },
      })
    )
    const { wrapper } = withClient()
    const { result } = renderHook(() => useReplyToThread("dashboard:d1"), {
      wrapper,
    })
    await act(() =>
      result.current.mutateAsync({
        threadId: "t1",
        body: "a",
        startRuns: false,
      })
    )
    const sent = await act(() =>
      result.current.mutateAsync({ threadId: "t1", body: "b", startRuns: true })
    )
    expect(posts).toEqual([
      { path: "/api/v1/threads/t1/messages", body: { body: "a" } },
      {
        path: "/api/v1/threads/t1/messages",
        body: { body: "b", start_runs: true },
      },
    ])
    expect(sent.run).toEqual(run)
  })
})
