import { screen, waitFor, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { RepairQueue } from "./repair-page"
import type { RepairItem } from "./repair-state"

function item(id: string, title: string, extra: Partial<RepairItem> = {}) {
  return {
    id,
    title,
    folder: { id: "f1", title: "Shop", general: false, personal: false },
    unverified_panels: 2,
    unverified_variables: 0,
    placeholder_panels: 1,
    last_import_at: "2026-11-06T09:00:00Z",
    repair: null,
    ...extra,
  } as unknown as RepairItem
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function serve(items: RepairItem[], agentAvailable: boolean) {
  const posted: unknown[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (url.pathname === "/api/v1/repairs" && request.method === "POST") {
        posted.push(await request.json())
        return json({ queued: [], skipped: [] })
      }
      if (url.pathname === "/api/v1/repairs") {
        return json({ dashboards: items, next_cursor: null })
      }
      if (url.pathname === "/api/v1/agent") {
        return json({ available: agentAvailable })
      }
      return json({ error: { code: "not_found", message: "no" } }, 404)
    })
  )
  return posted
}

afterEach(() => vi.unstubAllGlobals())

describe("RepairQueue", () => {
  it("lists each dashboard with its counts and queues a repair for the one asked", async () => {
    const posted = serve(
      [item("d1", "Checkout"), item("d2", "Cart", { unverified_panels: 0 })],
      true
    )
    const { user } = renderWithProviders(<RepairQueue folderId={null} />, {
      route: { path: "/", url: "/" },
    })

    const grid = await screen.findByRole("grid", {
      name: "Dashboards to repair",
    })
    const checkout = await within(grid).findByRole("link", { name: "Checkout" })
    expect(checkout).toBeVisible()
    expect(within(grid).getByText("2 panels")).toBeVisible()
    const buttons = within(grid).getAllByRole("button", {
      name: "Repair with the agent",
    })
    expect(buttons[1]).toHaveAttribute("aria-disabled", "true")

    await user.click(buttons[0])
    await waitFor(() => expect(posted).toEqual([{ dashboard_ids: ["d1"] }]))
  })

  it("disables every repair, with the reason, when no agent is configured", async () => {
    serve([item("d1", "Checkout")], false)
    renderWithProviders(<RepairQueue folderId={null} />, {
      route: { path: "/", url: "/" },
    })

    const button = await screen.findByRole("button", {
      name: "Repair with the agent",
    })
    expect(button).toHaveAttribute("aria-disabled", "true")
    expect(button).toHaveAccessibleDescription("No agent is configured")
  })

  it("says so when nothing needs repair", async () => {
    serve([], true)
    renderWithProviders(<RepairQueue folderId={null} />, {
      route: { path: "/", url: "/" },
    })

    expect(
      await screen.findByText(
        "No imported dashboard has failing panels or placeholders."
      )
    ).toBeVisible()
    expect(screen.queryByRole("grid")).toBeNull()
  })
})
