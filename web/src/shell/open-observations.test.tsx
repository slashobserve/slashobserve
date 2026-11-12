import { screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { OpenObservations } from "./open-observations"

const route = { path: "/o/$observationId", url: "/o/o2" }

function observation(id: string, title: string) {
  return { id, title, state: "open" }
}

function answer(observations: unknown[], nextCursor: string | null) {
  const fetch = vi.fn(
    async () =>
      new Response(JSON.stringify({ observations, next_cursor: nextCursor }), {
        status: 200,
        headers: { "content-type": "application/json" },
      })
  )
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

    const request = fetch.mock.calls[0] as unknown as [Request]
    const url = new URL(request[0].url, "http://localhost")
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

  it("renders nothing when the sidebar is folded to icons", async () => {
    answer([observation("o1", "Checkout latency")], null)
    renderWithProviders(<OpenObservations collapsed />, { route })

    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(screen.queryByRole("link")).not.toBeInTheDocument()
  })
})
