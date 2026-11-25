import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { ObservationList } from "./observation-list"
import { defaultView } from "./list-view"

const folderId = "5d0c7a2e-2f43-4a4e-9a55-0d0b8f3c1a77"

function observation(
  id: string,
  title: string,
  extra: Record<string, unknown> = {}
) {
  return {
    id,
    title,
    state: "open",
    folder_id: folderId,
    updated_at: "2026-11-06T09:00:00Z",
    created_by: {
      kind: "user",
      id: "u1",
      name: "Ana",
      origin: "web",
      client: null,
      run_id: null,
      label: "Ana",
    },
    allowed_actions: ["resolve", "delete"],
    ...extra,
  }
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
}

function serve(observations: unknown[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (url.pathname === "/api/v1/observations") {
        return json({ observations, total: observations.length })
      }
      if (url.pathname === "/api/v1/folders") {
        return json({
          folders: [{ id: folderId, title: "Shop", permissions: {} }],
        })
      }
      return json({})
    })
  )
}

function renderList() {
  return renderWithProviders(
    <ObservationList
      view={defaultView}
      creating={false}
      onChange={() => undefined}
    />,
    { route: { path: "/", url: "/" } }
  )
}

afterEach(() => vi.unstubAllGlobals())

describe("ObservationList", () => {
  it("lists each observation with its folder, state and maker", async () => {
    serve([
      observation("o1", "Checkout latency"),
      observation("o2", "Cart errors", { state: "resolved" }),
    ])
    renderList()

    const grid = await screen.findByRole("grid", { name: "Observations" })
    expect(
      await within(grid).findByRole("link", { name: "Checkout latency" })
    ).toHaveAttribute("href", "/o/o1")
    expect(within(grid).getAllByRole("link", { name: "Shop" })).toHaveLength(2)
    expect(within(grid).getByText("Open")).toBeInTheDocument()
    expect(within(grid).getByText("Resolved")).toBeInTheDocument()
    expect(within(grid).getAllByText("Ana")).toHaveLength(2)
  })

  it("selects every row from the header, offers the bulk actions and clears the selection", async () => {
    serve([observation("o1", "Checkout latency"), observation("o2", "Cart")])
    const { user } = renderList()

    const all = await screen.findByRole("checkbox", {
      name: "Select every observation on this page",
    })
    await user.click(all)

    const bar = await screen.findByRole("group", {
      name: "Selected observations",
    })
    expect(within(bar).getByText("2 selected")).toBeInTheDocument()
    expect(within(bar).getByRole("button", { name: "Resolve 2" })).toBeEnabled()
    expect(
      screen.getByRole("checkbox", { name: "Select Checkout latency" })
    ).toBeChecked()

    await user.click(
      screen.getByRole("checkbox", { name: "Select Checkout latency" })
    )
    await waitFor(() =>
      expect(within(bar).getByText("1 selected")).toBeInTheDocument()
    )
    expect(
      screen.getByRole("checkbox", {
        name: "Select every observation on this page",
      })
    ).toHaveAttribute("aria-checked", "mixed")

    await user.click(
      within(bar).getByRole("button", { name: "Clear selection" })
    )
    expect(
      screen.queryByRole("group", { name: "Selected observations" })
    ).toBeNull()
  })

  it("offers only the actions an observation allows", async () => {
    serve([
      observation("o1", "Checkout latency", { allowed_actions: ["reopen"] }),
      observation("o2", "Cart", { allowed_actions: [] }),
    ])
    const { user } = renderList()

    await user.click(
      await screen.findByRole("button", {
        name: "Actions for Checkout latency",
      })
    )
    expect(
      await screen.findByRole("menuitem", { name: "Reopen" })
    ).toBeInTheDocument()
    expect(screen.queryByRole("menuitem", { name: "Delete" })).toBeNull()
    expect(
      screen.queryByRole("button", { name: "Actions for Cart" })
    ).toBeNull()
  })
})
