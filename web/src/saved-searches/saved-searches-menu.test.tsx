import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { SavedSearchesMenu } from "./saved-searches-menu"
import { must } from "@/test/must"

const navigate = vi.hoisted(() => vi.fn())

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useNavigate: () => navigate,
}))

const viewerPermissions = {
  comment: false,
  edit: false,
  manage: false,
  create: false,
}

function saved(id: string, title: string, personal = false) {
  return {
    id,
    kind: "logs",
    title,
    datasource_uid: "es",
    query: "level:error",
    filters: { columns: ["host"] },
    from: "now-6h",
    to: "now",
    folder: { id: "f1", title: "Shop", general: false, personal },
    permissions: { comment: true, edit: true, manage: false, create: false },
  }
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function stubServer(items: unknown[]) {
  const requests: URL[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      requests.push(new URL(request.url))
      return json({ saved_searches: items, next_cursor: null })
    })
  )
  return requests
}

afterEach(() => {
  vi.unstubAllGlobals()
  navigate.mockClear()
})

describe("SavedSearchesMenu", () => {
  it("lists the saved searches of its kind with their folders", async () => {
    const requests = stubServer([
      saved("s1", "Checkout errors"),
      saved("s2", "Slow payments", true),
    ])
    const { user } = renderWithProviders(<SavedSearchesMenu kind="logs" />)
    await user.click(screen.getByRole("combobox", { name: "Saved searches" }))
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual([
      "Checkout errorsShop",
      "Slow paymentsMy Dashboards",
    ])
    expect(must(requests[0]).searchParams.get("kind")).toBe("logs")
  })

  it("opens and lists a search the caller may only view", async () => {
    stubServer([
      { ...saved("s1", "Checkout errors"), permissions: viewerPermissions },
    ])
    const { user } = renderWithProviders(<SavedSearchesMenu kind="logs" />)
    await user.click(screen.getByRole("combobox", { name: "Saved searches" }))
    expect(
      await screen.findByRole("option", { name: /Checkout errors/ })
    ).toBeVisible()
  })

  it("names the chevron and keeps it out of the tab order", () => {
    stubServer([])
    renderWithProviders(<SavedSearchesMenu kind="logs" />)
    const chevron = screen.getByRole("button", { name: "Show saved searches" })
    expect(chevron).toHaveAttribute("tabindex", "-1")
  })

  it("opens the chosen search on its page", async () => {
    stubServer([saved("s1", "Checkout errors")])
    const { user } = renderWithProviders(<SavedSearchesMenu kind="logs" />)
    await user.click(screen.getByRole("combobox", { name: "Saved searches" }))
    await user.click(
      await screen.findByRole("option", { name: /Checkout errors/ })
    )
    await waitFor(() =>
      expect(navigate).toHaveBeenCalledWith({
        to: "/logs/$datasourceUid",
        params: { datasourceUid: "es" },
        search: {
          q: "level:error",
          cols: "host",
          from: "now-6h",
          to: "now",
          saved: "s1",
        },
      })
    )
  })

  it("says there are none yet", async () => {
    stubServer([])
    const { user } = renderWithProviders(<SavedSearchesMenu kind="traces" />)
    await user.click(screen.getByRole("combobox", { name: "Saved searches" }))
    expect(await screen.findByText("No saved searches yet.")).toBeVisible()
  })
})
