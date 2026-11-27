import { afterEach, describe, expect, it, vi } from "vitest"

import { testQueryClient } from "@/test/render"

import { revisionsQuery } from "./queries"

afterEach(() => {
  vi.unstubAllGlobals()
})

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
}

describe("revisionsQuery", () => {
  it("follows next_cursor so revisions past the first page are listed", async () => {
    const fetchMock = vi.fn(async (request: Request) => {
      const cursor = new URL(request.url).searchParams.get("cursor")
      return cursor === null
        ? json({ revisions: [{ id: "r1" }], next_cursor: "page-2" })
        : json({ revisions: [{ id: "r2" }], next_cursor: null })
    })
    vi.stubGlobal("fetch", fetchMock)
    const revisions = await testQueryClient().fetchQuery(revisionsQuery("d1"))
    expect(revisions.map((revision) => revision.id)).toEqual(["r1", "r2"])
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})
