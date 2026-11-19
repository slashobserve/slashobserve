import { screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  DropdownMenu,
  DropdownMenuContent,
} from "@/components/ui/dropdown-menu"
import { HomeDashboardItem } from "@/dashboards/home-dashboard-item"
import { HomeDashboardSection } from "@/settings/home-dashboard-section"
import { renderWithProviders } from "@/test/render"

import { HomeDashboardNotice } from "./home-dashboard-notice"

const toastError = vi.hoisted(() => vi.fn())

vi.mock("sonner", () => ({ toast: { error: toastError } }))

const none = { status: "none", dashboard: null, set_at: null }

function chosen(id: string) {
  return {
    status: "ok",
    dashboard: {
      id,
      title: "Board " + id,
      folder_id: "f",
      folder_title: "Ops",
    },
    set_at: "2026-11-19T00:00:00Z",
  }
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  })
}

function serve(initial: unknown) {
  let current = initial
  const writes: string[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url)
      if (!pathname.endsWith("/me/home-dashboard")) return json({}, 404)
      if (request.method === "GET") return json(current)
      writes.push(request.method)
      if (request.method === "DELETE") {
        current = none
        return new Response(null, { status: 204 })
      }
      const body = (await request.json()) as { dashboard_id: string }
      current = chosen(body.dashboard_id)
      return json(current)
    })
  )
  return writes
}

beforeEach(() => {
  toastError.mockReset()
  return () => vi.unstubAllGlobals()
})

function item(id: string) {
  return renderWithProviders(
    <DropdownMenu open>
      <DropdownMenuContent>
        <HomeDashboardItem dashboardId={id} />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

describe("HomeDashboardItem", () => {
  it("chooses the dashboard, then offers to stop", async () => {
    const writes = serve(none)
    const { user } = item("d1")
    const choose = await screen.findByRole("menuitem", {
      name: "Open on sign-in",
    })
    await waitFor(() =>
      expect(choose).not.toHaveAttribute("aria-disabled", "true")
    )
    await user.click(choose)
    expect(writes).toEqual(["PUT"])
  })

  it("stops opening the dashboard that is the home dashboard", async () => {
    const writes = serve(chosen("d1"))
    const { user } = item("d1")
    const stop = await screen.findByRole("menuitem", {
      name: "Stop opening on sign-in",
    })
    await user.click(stop)
    expect(writes).toEqual(["DELETE"])
  })

  it("offers to choose when another dashboard is the home dashboard", async () => {
    serve(chosen("d2"))
    item("d1")
    expect(
      await screen.findByRole("menuitem", { name: "Open on sign-in" })
    ).toBeInTheDocument()
  })
})

describe("HomeDashboardNotice", () => {
  it.each([
    ["gone", /was deleted/],
    ["unreadable", /no longer view/],
  ])(
    "tells the person when the dashboard is %s and clears the choice",
    async (status, text) => {
      const writes = serve({
        status,
        dashboard: null,
        set_at: "2026-11-19T00:00:00Z",
      })
      const { user } = renderWithProviders(<HomeDashboardNotice />)
      expect(await screen.findByRole("status")).toHaveTextContent(text)
      await user.click(screen.getByRole("button", { name: "Clear it" }))
      await waitFor(() => expect(screen.queryByRole("status")).toBeNull())
      expect(writes).toEqual(["DELETE"])
    }
  )

  it("says nothing when none is set or it opens", async () => {
    serve(chosen("d1"))
    const { unmount } = renderWithProviders(<HomeDashboardNotice />)
    await waitFor(() => expect(fetch).toHaveBeenCalled())
    expect(screen.queryByRole("status")).toBeNull()
    unmount()
  })
})

describe("HomeDashboardSection", () => {
  it("says none is chosen", async () => {
    serve(none)
    renderWithProviders(<HomeDashboardSection />, {
      route: { path: "/", url: "/" },
    })
    expect(await screen.findByText(/None chosen/)).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Clear" })).toBeNull()
  })

  it("links the chosen dashboard and clears it", async () => {
    const writes = serve(chosen("d1"))
    const { user } = renderWithProviders(<HomeDashboardSection />, {
      route: { path: "/", url: "/" },
    })
    expect(
      await screen.findByRole("link", { name: "Board d1" })
    ).toBeInTheDocument()
    expect(screen.getByText(/in Ops/)).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Clear" }))
    expect(await screen.findByText(/None chosen/)).toBeInTheDocument()
    expect(writes).toEqual(["DELETE"])
  })

  it("explains a dashboard that was deleted", async () => {
    serve({ status: "gone", dashboard: null, set_at: "2026-11-19T00:00:00Z" })
    renderWithProviders(<HomeDashboardSection />, {
      route: { path: "/", url: "/" },
    })
    expect(await screen.findByText(/was deleted/)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Clear" })).toBeInTheDocument()
  })
})
