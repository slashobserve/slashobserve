import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import type { AgentRef } from "./agent-ref"
import { RecentWork } from "./recent-work"

type Item = {
  id: string
  title: string
  state: string
  openable: boolean
  requested_by: { kind: string; name: string }
  dashboard: { id: string; title: string } | null
  observation: { id: string; title: string } | null
}

function item(overrides: Partial<Item> & { id: string }): Item {
  return {
    title: "Why is checkout slow?",
    state: "succeeded",
    openable: true,
    requested_by: { kind: "user", name: "Ada" },
    dashboard: null,
    observation: null,
    ...overrides,
  }
}

function page(items: Item[], scope = "you", nextCursor: string | null = null) {
  return {
    scope,
    next_cursor: nextCursor,
    items: items.map((entry) => ({
      source: "prompt",
      result_kind: "none",
      failure_code: null,
      created_at: new Date(Date.now() - 3 * 60_000).toISOString(),
      started_at: null,
      finished_at: null,
      cost_usd: null,
      ...entry,
    })),
  }
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function serve(reply: (url: URL) => Response) {
  const fetch = vi.fn(async (request: Request) => {
    const url = new URL(request.url)
    return url.pathname === "/api/v1/agents/runs"
      ? reply(url)
      : json({ error: "unexpected", code: "unexpected" }, 500)
  })
  vi.stubGlobal("fetch", fetch)
  return fetch
}

function renderWork(agentRef: AgentRef) {
  return renderWithProviders(<RecentWork agentRef={agentRef} />, {
    route: { path: "/", url: "/" },
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("RecentWork", () => {
  it("lists each run with its title, state word and time", async () => {
    serve(() =>
      json(
        page([
          item({ id: "r1", title: "Why is checkout slow?", state: "failed" }),
          item({ id: "r2", title: "Add a latency panel", state: "canceling" }),
        ])
      )
    )
    renderWork({ kind: "observe" })

    expect(
      await screen.findByRole("heading", { level: 2, name: "Recent work" })
    ).toBeTruthy()
    expect(await screen.findByText("Why is checkout slow?")).toBeTruthy()
    expect(screen.getByText("Failed")).toBeTruthy()
    expect(screen.getByText("Stopping")).toBeTruthy()
    expect(screen.getAllByText("3 min ago")).toHaveLength(2)
  })

  it("links only the runs the person asked for", async () => {
    serve(() =>
      json(
        page([
          item({ id: "mine", title: "My run", openable: true }),
          item({ id: "theirs", title: "Their run", openable: false }),
        ])
      )
    )
    renderWork({ kind: "observe" })

    const mine = await screen.findByRole("link", { name: "My run" })
    expect(mine.getAttribute("href")).toBe("/c/mine")
    expect(screen.getByText("Their run")).toBeTruthy()
    expect(screen.queryByRole("link", { name: "Their run" })).toBeNull()
  })

  it("links the dashboard and the observation a run touched", async () => {
    serve(() =>
      json(
        page([
          item({
            id: "r1",
            dashboard: { id: "d1", title: "Checkout" },
            observation: { id: "o1", title: "Slow checkout" },
          }),
        ])
      )
    )
    renderWork({ kind: "observe" })

    expect(
      (await screen.findByRole("link", { name: "Checkout" })).getAttribute(
        "href"
      )
    ).toBe("/d/d1/checkout")
    expect(
      screen.getByRole("link", { name: "Slow checkout" }).getAttribute("href")
    ).toBe("/o/o1")
  })

  it("names who asked only in the workspace scope", async () => {
    const bob = { kind: "user", name: "Bob" }
    let scope = "workspace"
    serve(() => json(page([item({ id: "r1", requested_by: bob })], scope)))
    const first = renderWork({ kind: "observe" })
    expect(await screen.findByText("by Bob")).toBeTruthy()
    first.unmount()

    scope = "you"
    renderWork({ kind: "observe" })
    await screen.findByText("Why is checkout slow?")
    expect(screen.queryByText("by Bob")).toBeNull()
  })

  it("says there are no runs yet for an empty page", async () => {
    serve(() => json(page([])))
    renderWork({ kind: "grant", id: "g1" })

    expect(await screen.findByText("No runs yet.")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull()
  })

  it("loads the next page with the cursor and appends its rows", async () => {
    const fetch = serve((url) =>
      url.searchParams.get("cursor") === "next-1"
        ? json(page([item({ id: "r2", title: "Second page run" })]))
        : json(
            page([item({ id: "r1", title: "First page run" })], "you", "next-1")
          )
    )
    const { user } = renderWork({ kind: "observe" })

    await screen.findByText("First page run")
    await user.click(screen.getByRole("button", { name: "Load more" }))

    expect(await screen.findByText("Second page run")).toBeTruthy()
    expect(screen.getByText("First page run")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull()
    const cursors = fetch.mock.calls.map(([request]) =>
      new URL(request.url).searchParams.get("cursor")
    )
    expect(cursors).toEqual([null, "next-1"])
  })

  it("asks for the built-in agent without an id and for a grant with its id", async () => {
    const fetch = serve(() => json(page([])))
    const observe = renderWork({ kind: "observe" })
    await screen.findByText("No runs yet.")
    observe.unmount()
    renderWork({ kind: "service_account", id: "s1" })
    await screen.findByText("No runs yet.")

    const queries = fetch.mock.calls.map(
      ([request]) => new URL(request.url).searchParams
    )
    expect(queries[0]?.get("kind")).toBe("builtin")
    expect(queries[0]?.has("id")).toBe(false)
    expect(queries[1]?.get("kind")).toBe("service_account")
    expect(queries[1]?.get("id")).toBe("s1")
  })

  it("shows a problem with a retry when the runs cannot load", async () => {
    serve(() => json({ error: "internal error", code: "internal" }, 500))
    renderWork({ kind: "observe" })

    expect(await screen.findByText("Could not load recent work")).toBeTruthy()
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy()
  })
})
