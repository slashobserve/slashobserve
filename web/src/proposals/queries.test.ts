import { QueryClient } from "@tanstack/react-query"
import { afterEach, describe, expect, it, vi } from "vitest"

import { dashboardProposalsQuery, runProposalQuery } from "./queries"

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

describe("runProposalQuery", () => {
  it("asks for the run's proposal in any state instead of scanning a page", async () => {
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
    await new QueryClient().fetchQuery(runProposalQuery("d1", "r1"))
    const url = new URL(urls[0] ?? "")
    expect(url.searchParams.get("run_id")).toBe("r1")
    expect(url.searchParams.get("state")).toBe("all")
    expect(url.searchParams.get("limit")).toBe("1")
  })

  it("is cleared with the other proposal queries", () => {
    expect(runProposalQuery("d1", "r1").queryKey.slice(0, 1)).toEqual([
      "proposals",
    ])
  })
})
