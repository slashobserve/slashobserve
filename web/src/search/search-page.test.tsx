import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { SearchPage } from "./search-page"

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function hit(kind: string, id: string, title: string, extra = {}) {
  return {
    kind,
    id,
    title,
    snippet: [
      { text: "see ", match: false },
      { text: "[the **latency**](https://x.test)", match: true },
    ],
    url: `https://observe.example.com/d/${id}/slug`,
    folder: { id: "f1", title: "Payments" },
    target: null,
    thread_id: null,
    updated_at: "2026-11-06T09:00:00Z",
    ...extra,
  }
}

function mockSearch(pages: unknown[]) {
  const requested: URL[] = []
  let call = 0
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      requested.push(new URL(request.url))
      return pages[Math.min(call++, pages.length - 1)] instanceof Response
        ? (pages[Math.min(call - 1, pages.length - 1)] as Response)
        : json(pages[Math.min(call - 1, pages.length - 1)])
    })
  )
  return requested
}

function show(q: string, kind: "annotation" | null = null) {
  const onChange = vi.fn()
  const view = renderWithProviders(
    <SearchPage q={q} kind={kind} onChange={onChange} />,
    { route: { path: "/search", url: "/search" } }
  )
  return { ...view, onChange }
}

afterEach(() => vi.unstubAllGlobals())

describe("SearchPage", () => {
  it("explains the syntax when there is no query and asks nothing", async () => {
    const requested = mockSearch([])
    show("")
    expect(await screen.findByText("Search the workspace")).toBeTruthy()
    expect(requested).toHaveLength(0)
  })

  it("lists results with plain-text snippets and links a dashboard", async () => {
    mockSearch([
      {
        results: [hit("dashboard", "d1", "Checkout latency")],
        next_cursor: null,
      },
    ])
    show("latency")
    const link = await screen.findByRole("link", { name: /Checkout latency/ })
    expect(link.getAttribute("href")).toContain("/d/d1")
    expect(within(link).getByText("the latency")).toBeTruthy()
    expect(link.textContent).not.toContain("**")
  })

  it("shows a global annotation as a row with no link", async () => {
    mockSearch([
      {
        results: [hit("annotation", "n1", "Deploy", { url: null })],
        next_cursor: null,
      },
    ])
    show("deploy")
    expect(await screen.findByText("Deploy")).toBeTruthy()
    expect(
      screen.getByText("On every dashboard, no page of its own")
    ).toBeTruthy()
    expect(screen.queryByRole("link", { name: /Deploy/ })).toBeNull()
  })

  it("loads the next page with the cursor", async () => {
    const requested = mockSearch([
      { results: [hit("dashboard", "d1", "First")], next_cursor: "c2" },
      { results: [hit("dashboard", "d2", "Second")], next_cursor: null },
    ])
    const { user } = show("latency")
    await user.click(await screen.findByRole("button", { name: "Show more" }))
    await screen.findByText("Second")
    expect(requested[1]?.searchParams.get("cursor")).toBe("c2")
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull()
  })

  it("narrows to one kind and reports the choice", async () => {
    mockSearch([
      { results: [hit("dashboard", "d1", "First")], next_cursor: null },
    ])
    const { user, onChange } = show("latency")
    await user.click(await screen.findByRole("button", { name: "Comments" }))
    expect(onChange).toHaveBeenCalledWith({ q: "latency", kind: "comment" })
  })

  it("sends the kind filter to the server", async () => {
    const requested = mockSearch([{ results: [], next_cursor: null }])
    show("deploy", "annotation")
    await waitFor(() => expect(requested).toHaveLength(1))
    expect(requested[0]?.searchParams.getAll("kind")).toEqual(["annotation"])
  })

  it("says that misspellings are not matched when nothing is found", async () => {
    mockSearch([{ results: [], next_cursor: null }])
    show("chekout")
    expect(await screen.findByText(/Nothing matches "chekout"/)).toBeTruthy()
    expect(screen.getByText(/not misspellings/)).toBeTruthy()
  })

  it("shows the server's message for a query it rejects", async () => {
    mockSearch([
      json({ error: "Unbalanced quote", code: "invalid_query" }, 400),
    ])
    show('"checkout')
    expect(await screen.findByText("Unbalanced quote")).toBeTruthy()
  })
})
