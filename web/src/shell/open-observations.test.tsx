import { screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { OpenObservations } from "./open-observations"

const route = { path: "/o/$observationId", url: "/o/o2" }

function observation(id: string, title: string) {
  return { id, title, state: "open" }
}

function inboxItem(observationId: string, unread: boolean) {
  return {
    seq: 1,
    type: "observation.block_added",
    target: { kind: "observation", id: observationId },
    reason: "requested_block",
    unread,
    data: {},
  }
}

function answer(
  observations: unknown[],
  nextCursor: string | null,
  inbox: unknown[] = []
) {
  const fetch = vi.fn(async (request: Request) => {
    const body = new URL(request.url).pathname.endsWith("/inbox")
      ? { items: inbox, next_cursor: null, unread_count: inbox.length }
      : { observations, next_cursor: nextCursor }
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { "content-type": "application/json" },
    })
  })
  vi.stubGlobal("fetch", fetch)
  return fetch
}

beforeEach(() => {
  return () => vi.unstubAllGlobals()
})

describe("OpenObservations", () => {
  it("lists each open observation as a link to its page, marking the current one", async () => {
    const fetch = answer(
      [observation("o1", "Checkout latency"), observation("o2", "Slow pods")],
      null
    )
    renderWithProviders(<OpenObservations collapsed={false} />, { route })

    const first = await screen.findByRole("link", { name: "Checkout latency" })
    expect(first).toHaveAttribute("href", "/o/o1")
    expect(first).not.toHaveAttribute("aria-current")
    expect(screen.getByRole("link", { name: "Slow pods" })).toHaveAttribute(
      "aria-current",
      "page"
    )
    expect(
      screen.getByRole("group", { name: "Open observations 2" })
    ).toBeInTheDocument()

    const request = fetch.mock.calls
      .map(([call]) => call)
      .find((call) => new URL(call.url).pathname.endsWith("/observations"))
    const url = new URL(request!.url, "http://localhost")
    expect(url.searchParams.get("state")).toBe("open")
    expect(url.searchParams.get("limit")).toBe("8")
  })

  it("shows a plus after the count when more are open", async () => {
    answer(
      Array.from({ length: 8 }, (_, index) =>
        observation(`o${index}`, `Observation ${index}`)
      ),
      "next"
    )
    renderWithProviders(<OpenObservations collapsed={false} />, { route })

    expect(
      await screen.findByRole("group", { name: "Open observations 8+" })
    ).toBeInTheDocument()
    expect(screen.getAllByRole("link")).toHaveLength(8)
  })

  it("renders nothing when no observation is open", async () => {
    const fetch = answer([], null)
    const { container } = renderWithProviders(
      <OpenObservations collapsed={false} />,
      { route }
    )

    await vi.waitFor(() => expect(fetch).toHaveBeenCalled())
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(screen.queryByText("Open observations")).not.toBeInTheDocument()
    expect(container.querySelector("ul")).toBeNull()
  })

  it("marks an observation with an unread inbox item and leaves the others plain", async () => {
    answer(
      [observation("o1", "Checkout latency"), observation("o2", "Slow pods")],
      null,
      [inboxItem("o1", true), inboxItem("o2", false)]
    )
    renderWithProviders(<OpenObservations collapsed={false} />, { route })

    const unread = await screen.findByRole("link", {
      name: "Checkout latency Unread",
    })
    expect(unread.querySelector(".font-semibold")).not.toBeNull()
    const read = screen.getByRole("link", { name: "Slow pods" })
    expect(read.querySelector(".font-semibold")).toBeNull()
    expect(screen.getAllByText("Unread")).toHaveLength(1)
  })

  it("renders nothing when the sidebar is folded to icons", async () => {
    answer([observation("o1", "Checkout latency")], null)
    renderWithProviders(<OpenObservations collapsed />, { route })

    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(screen.queryByRole("link")).not.toBeInTheDocument()
  })
})
