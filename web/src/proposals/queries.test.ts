import { QueryClient } from "@tanstack/react-query"
import { afterEach, describe, expect, it, vi } from "vitest"

import { dashboardProposalsQuery } from "./queries"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("dashboardProposalsQuery", () => {
  it("asks for the first 50 open proposals of the dashboard by default", async () => {
    const urls: string[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn((request: Request) => {
        urls.push(request.url)
        return Promise.resolve(
          new Response(JSON.stringify({ proposals: [], next_cursor: null }), {
            headers: { "Content-Type": "application/json" },
          })
        )
      })
    )
    const client = new QueryClient()
    const list = await client.fetchQuery(dashboardProposalsQuery("d1"))
    expect(list.proposals).toEqual([])
    const url = new URL(urls[0] ?? "")
    expect(url.pathname).toBe("/api/v1/dashboards/d1/proposals")
    expect(url.searchParams.get("state")).toBe("open")
    expect(url.searchParams.get("limit")).toBe("50")
  })

  it("keys each state separately", () => {
    expect(dashboardProposalsQuery("d1", "merged").queryKey).not.toEqual(
      dashboardProposalsQuery("d1").queryKey
    )
  })
})
