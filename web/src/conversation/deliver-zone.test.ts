import { QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook } from "@testing-library/react"
import { createElement, type ReactNode } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { testQueryClient } from "@/test/render"

import { useDeliver } from "./use-composer"

const threadId = "6f1e2d3c-4b5a-4968-8776-655443322110"
const browserZone = Intl.DateTimeFormat().resolvedOptions().timeZone

function stubPosts() {
  const posts: { path: string; body: Record<string, unknown> }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      posts.push({
        path: new URL(request.url).pathname,
        body: await request.clone().json(),
      })
      return new Response("{}", {
        status: 500,
        headers: { "Content-Type": "application/json" },
      })
    })
  )
  return posts
}

function wrapper({ children }: { children: ReactNode }) {
  return createElement(
    QueryClientProvider,
    { client: testQueryClient() },
    children
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("useDeliver", () => {
  it("sends the browser time zone with a thread's first message and a reply", async () => {
    const posts = stubPosts()
    const { result } = renderHook(
      () => useDeliver({ kind: "observation", id: "o1" }),
      { wrapper }
    )
    await act(() =>
      result.current
        .deliver({
          via: "thread.open",
          target: "observation:o1",
          body: "why",
          to: "agent",
          clientId: "c1",
        })
        .catch(() => undefined)
    )
    await act(() =>
      result.current
        .deliver({
          via: "thread.reply",
          threadId,
          body: "and now",
          to: "agent",
          clientId: "c2",
        })
        .catch(() => undefined)
    )
    expect(posts.map((post) => post.body.time_zone)).toEqual([
      browserZone,
      browserZone,
    ])
  })

  it("sends the zone the composer names instead", async () => {
    const posts = stubPosts()
    const { result } = renderHook(
      () => useDeliver({ kind: "observation", id: "o1" }, "Asia/Kolkata"),
      { wrapper }
    )
    await act(() =>
      result.current
        .deliver({
          via: "thread.reply",
          threadId,
          body: "and now",
          clientId: "c2",
        })
        .catch(() => undefined)
    )
    expect(posts[0]?.body.time_zone).toBe("Asia/Kolkata")
  })
})
