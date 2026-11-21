import { screen, within } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AuditFacets } from "./audit-facets"

const fetchMock = vi.hoisted(() => vi.fn())

const window = {
  q: "origin:mcp",
  since: "2026-11-21T05:00:00.000Z",
  until: "2026-11-21T07:00:00.000Z",
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
  })
}

beforeEach(() => {
  fetchMock.mockReset()
  fetchMock.mockImplementation((request: Request) => {
    const url = new URL(request.url)
    if (url.pathname.endsWith("/audit/fields")) return json({ fields: [] })
    return json({
      field: url.searchParams.get("field"),
      values:
        url.searchParams.get("field") === "outcome"
          ? [{ value: "denied", count: 2 }]
          : [],
      form: "kql",
    })
  })
  vi.stubGlobal("fetch", fetchMock)
  return () => vi.unstubAllGlobals()
})

describe("AuditFacets", () => {
  it("toggles a chip against the query text the person sees", async () => {
    const onQuery = vi.fn()
    const { user } = renderWithProviders(
      <AuditFacets
        window={window}
        query="origin:mcp and actor:bob"
        live={[]}
        onQuery={onQuery}
      />
    )
    const group = await screen.findByRole("group", { name: "Outcome values" })
    await user.click(
      await within(group).findByRole("button", { name: "denied 2" })
    )
    expect(onQuery).toHaveBeenCalledWith(
      'origin:mcp and actor:bob and outcome:"denied"'
    )
  })
})
