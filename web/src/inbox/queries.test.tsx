import type { ReactNode } from "react"
import { QueryClientProvider } from "@tanstack/react-query"
import { renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest"

import { inboxKeys } from "@/api/keys"
import { testQueryClient } from "@/test/render"

import {
  inboxPollInterval,
  InboxStreamContext,
  useInboxForYou,
} from "./queries"

describe("inboxPollInterval", () => {
  test("polls every 15 s outside the shell while the page is visible", () => {
    expect(inboxPollInterval(false, true)).toBe(15_000)
  })

  test("never polls inside the shell, whose activity stream keeps it live", () => {
    expect(inboxPollInterval(true, true)).toBe(false)
    expect(inboxPollInterval(true, false)).toBe(false)
  })

  test("never polls while the page is hidden", () => {
    expect(inboxPollInterval(false, false)).toBe(false)
  })
})

describe("useInboxForYou", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ items: [], next_cursor: null, unread_count: 0 }),
            { status: 200, headers: { "Content-Type": "application/json" } }
          )
      )
    )
  })
  afterEach(() => vi.unstubAllGlobals())

  function intervalWhen(streamed: boolean) {
    const client = testQueryClient()
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>
        <InboxStreamContext value={streamed}>{children}</InboxStreamContext>
      </QueryClientProvider>
    )
    renderHook(() => useInboxForYou(), { wrapper })
    const query = client
      .getQueryCache()
      .find({ queryKey: inboxKeys.list("for_you", "open") })
    return query?.observers[0]?.options.refetchInterval
  }

  test("polls on a page outside the shell and not inside it", () => {
    expect(intervalWhen(false)).toBe(15_000)
    expect(intervalWhen(true)).toBe(false)
  })
})
