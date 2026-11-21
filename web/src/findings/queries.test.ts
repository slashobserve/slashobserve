import { createElement, type ReactNode } from "react"
import { QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { chatKeys } from "@/api/keys"
import { testQueryClient } from "@/test/render"

import { useMakeDashboard } from "./queries"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("useMakeDashboard", () => {
  it("marks the chats stale so a send right after is planned from the attached chat", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response("{}", {
            status: 201,
            headers: { "Content-Type": "application/json" },
          })
      )
    )
    const queryClient = testQueryClient()
    queryClient.setQueryData(chatKeys.detail("c1"), { chat: { id: "c1" } })
    queryClient.setQueryData(chatKeys.list({}, 20), { pages: [] })
    const { result } = renderHook(() => useMakeDashboard("f1"), {
      wrapper: ({ children }: { children: ReactNode }) =>
        createElement(QueryClientProvider, { client: queryClient }, children),
    })
    await act(() => result.current.mutateAsync())
    expect(
      queryClient.getQueryState(chatKeys.detail("c1"))?.isInvalidated
    ).toBe(true)
    expect(
      queryClient.getQueryState(chatKeys.list({}, 20))?.isInvalidated
    ).toBe(true)
  })
})
