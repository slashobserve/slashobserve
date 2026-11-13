import { screen, waitFor, within } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { PinnedDashboards, useVisiblePins } from "./pinned-dashboards"
import { PinToggle } from "./pin-toggle"
import type { Pin } from "./pins"

function pin(id: string, title: string): Pin {
  return {
    id,
    title,
    folder_id: "f1",
    folder_title: "General",
    starred_at: "2026-11-06T09:00:00.000Z",
  }
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  })
}

function serve(initial: Pin[], dashboard = { id: "d9", title: "Checkout" }) {
  let items = initial
  const calls: string[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url)
      calls.push(`${request.method} ${pathname}`)
      if (pathname.endsWith("/stars")) return json({ items })
      if (pathname.endsWith("/star")) {
        const id = pathname.split("/").at(-2)!
        items =
          request.method === "PUT"
            ? [pin(id, dashboard.title), ...items]
            : items.filter((item) => item.id !== id)
        return new Response(null, { status: 204 })
      }
      return json({
        dashboard: {
          id: dashboard.id,
          title: dashboard.title,
          folder: { id: "f1", title: "General" },
        },
        version: { version: 1 },
      })
    })
  )
  return calls
}

beforeEach(() => {
  return () => vi.unstubAllGlobals()
})

function PinnedList({ currentId = null }: { currentId?: string | null }) {
  return <PinnedDashboards {...useVisiblePins()} currentId={currentId} />
}

const route = { path: "/d/$dashboardId/{-$slug}", url: "/d/d2/slow-pods" }

describe("PinnedDashboards", () => {
  it("lists pins in server order as links in an indented, labelled list, marking the open one", async () => {
    serve([pin("d1", "Checkout latency"), pin("d2", "Slow pods")])
    renderWithProviders(<PinnedList currentId="d2" />, { route })

    const list = await screen.findByRole("list", { name: "Pinned dashboards" })
    const links = within(list).getAllByRole("link")
    expect(links.map((link) => link.textContent)).toEqual([
      "Checkout latency",
      "Slow pods",
    ])
    expect(links[0]).toHaveAttribute("href", "/d/d1/checkout-latency")
    expect(links[0]).not.toHaveAttribute("aria-current")
    expect(links[1]).toHaveAttribute("aria-current", "page")
    expect(list.className).toContain("ml-[18px]")
  })

  it("gives each link its full title as a tooltip", async () => {
    const title = "A very long dashboard title that the sidebar must truncate"
    serve([pin("d1", title)])
    renderWithProviders(<PinnedList />, { route })

    expect(await screen.findByRole("link", { name: title })).toHaveAttribute(
      "title",
      title
    )
  })

  it("shows ten pins and a More link to the dashboards page when there are more", async () => {
    serve(
      Array.from({ length: 12 }, (_, index) =>
        pin(`d${index}`, `Dashboard ${index}`)
      )
    )
    renderWithProviders(<PinnedList />, { route })

    const more = await screen.findByRole("link", {
      name: "More pinned dashboards",
    })
    expect(more).toHaveAttribute("href", "/dashboards")
    const list = screen.getByRole("list", { name: "Pinned dashboards" })
    expect(within(list).getAllByRole("link")).toHaveLength(11)
    expect(screen.queryByRole("link", { name: "Dashboard 10" })).toBeNull()
  })

  it("shows no More link at ten pins", async () => {
    serve(
      Array.from({ length: 10 }, (_, index) =>
        pin(`d${index}`, `Dashboard ${index}`)
      )
    )
    renderWithProviders(<PinnedList />, { route })

    await screen.findByRole("link", { name: "Dashboard 9" })
    expect(screen.queryByRole("link", { name: /^More/ })).toBeNull()
  })

  it("renders nothing without pins", async () => {
    const calls = serve([])
    renderWithProviders(<PinnedList />, { route })

    await waitFor(() => expect(calls).toContain("GET /api/v1/stars"))
    expect(screen.queryByRole("list")).toBeNull()
  })

  it("follows the pin toggle: a pin appears first at once and leaves when unpinned", async () => {
    serve([pin("d1", "Checkout latency")])
    const { user } = renderWithProviders(
      <>
        <PinToggle dashboardId="d9" />
        <PinnedList />
      </>,
      { route }
    )

    const list = await screen.findByRole("list", { name: "Pinned dashboards" })
    const toggle = screen.getByRole("button", { name: "Pin to sidebar" })
    await waitFor(() => expect(toggle).toBeEnabled())
    expect(toggle).toHaveAttribute("aria-pressed", "false")

    await user.click(toggle)
    await waitFor(() =>
      expect(
        within(list)
          .getAllByRole("link")
          .map((link) => link.textContent)
      ).toEqual(["Checkout", "Checkout latency"])
    )
    const pressed = screen.getByRole("button", { name: "Unpin from sidebar" })
    expect(pressed).toHaveAttribute("aria-pressed", "true")

    await user.click(pressed)
    await waitFor(() =>
      expect(
        within(list)
          .getAllByRole("link")
          .map((link) => link.textContent)
      ).toEqual(["Checkout latency"])
    )
    expect(
      screen.getByRole("button", { name: "Pin to sidebar" })
    ).toHaveAttribute("aria-pressed", "false")
  })
})
