import { screen, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { FolderObservations } from "./folder-observations"

const folderId = "5d0c7a2e-2f43-4a4e-9a55-0d0b8f3c1a77"

function observation(id: string, title: string, state: "open" | "resolved") {
  return { id, title, state, updated_at: "2026-11-06T09:00:00Z" }
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function serve(
  pages: { observations: unknown[]; next_cursor: string | null }[]
) {
  const requested: URL[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      requested.push(url)
      const page = pages[url.searchParams.has("cursor") ? 1 : 0]
      return json(page)
    })
  )
  return requested
}

function renderSection() {
  return renderWithProviders(<FolderObservations folderId={folderId} />, {
    route: { path: "/", url: "/" },
  })
}

afterEach(() => vi.unstubAllGlobals())

describe("FolderObservations", () => {
  it("lists the folder's observations as links to their pages", async () => {
    const requested = serve([
      {
        observations: [
          observation("o1", "Checkout latency", "open"),
          observation("o2", "Cart errors", "resolved"),
        ],
        next_cursor: null,
      },
    ])
    renderSection()

    const region = await screen.findByRole("region", { name: "Observations" })
    expect(
      within(region).getByRole("link", { name: "Checkout latency" })
    ).toHaveAttribute("href", "/o/o1")
    expect(
      within(region).getByRole("link", { name: "Cart errors" })
    ).toHaveAttribute("href", "/o/o2")
    expect(within(region).getByText("Open")).toBeInTheDocument()
    expect(within(region).getByText("Resolved")).toBeInTheDocument()
    expect(requested[0]?.searchParams.get("folder_id")).toBe(folderId)
    expect(requested[0]?.searchParams.has("state")).toBe(false)
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull()
  })

  it("says so when the folder has no observations", async () => {
    serve([{ observations: [], next_cursor: null }])
    renderSection()

    expect(
      await screen.findByText("No observations in this folder")
    ).toBeInTheDocument()
    expect(screen.getByRole("heading", { name: "Observations" })).toBeVisible()
    expect(screen.queryByRole("link")).toBeNull()
  })

  it("loads the next page on request", async () => {
    serve([
      {
        observations: [observation("o1", "Checkout latency", "open")],
        next_cursor: "c1",
      },
      {
        observations: [observation("o2", "Cart errors", "open")],
        next_cursor: null,
      },
    ])
    const { user } = renderSection()

    await user.click(await screen.findByRole("button", { name: "Load more" }))

    expect(
      await screen.findByRole("link", { name: "Cart errors" })
    ).toHaveAttribute("href", "/o/o2")
    expect(screen.getByRole("link", { name: "Checkout latency" })).toBeVisible()
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull()
  })

  it("shows a retryable problem when the list fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        json({ error: { code: "internal", message: "boom" } }, 500)
      )
    )
    renderSection()

    expect(
      await screen.findByText("Could not load this folder's observations")
    ).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Retry" })).toBeVisible()
  })
})
