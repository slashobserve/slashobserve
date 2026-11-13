import { screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { PinToggle } from "./pin-toggle"

const toastError = vi.hoisted(() => vi.fn())

vi.mock("sonner", () => ({ toast: { error: toastError } }))

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  })
}

function serve(initiallyPinned: boolean, writeStatus: number) {
  let pinned = initiallyPinned
  const writes: string[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url)
      if (pathname.endsWith("/stars")) {
        return json({
          items: pinned
            ? [
                {
                  id: "d1",
                  title: "Checkout",
                  folder_id: "f1",
                  folder_title: "General",
                  starred_at: "2026-11-06T09:00:00.000Z",
                },
              ]
            : [],
        })
      }
      if (pathname.endsWith("/star")) {
        writes.push(`${request.method} ${pathname}`)
        if (writeStatus === 204) pinned = request.method === "PUT"
        return writeStatus === 204
          ? new Response(null, { status: 204 })
          : json({ error: { code: "internal", message: "boom" } }, writeStatus)
      }
      return json({
        dashboard: {
          id: "d1",
          title: "Checkout",
          folder: { id: "f1", title: "General" },
        },
        version: { version: 1 },
      })
    })
  )
  return writes
}

beforeEach(() => {
  toastError.mockReset()
  return () => vi.unstubAllGlobals()
})

describe("PinToggle", () => {
  it("pins an unpinned dashboard with PUT and reads as pressed", async () => {
    const writes = serve(false, 204)
    const { user } = renderWithProviders(<PinToggle dashboardId="d1" />)

    const toggle = await screen.findByRole("button", {
      name: "Pin to sidebar",
    })
    await waitFor(() => expect(toggle).toBeEnabled())
    await user.click(toggle)

    expect(
      await screen.findByRole("button", { name: "Unpin from sidebar" })
    ).toHaveAttribute("aria-pressed", "true")
    expect(writes).toEqual(["PUT /api/v1/dashboards/d1/star"])
  })

  it("unpins a pinned dashboard with DELETE", async () => {
    const writes = serve(true, 204)
    const { user } = renderWithProviders(<PinToggle dashboardId="d1" />)

    const toggle = await screen.findByRole("button", {
      name: "Unpin from sidebar",
    })
    await waitFor(() => expect(toggle).toBeEnabled())
    expect(toggle).toHaveAttribute("aria-pressed", "true")
    await user.click(toggle)

    await screen.findByRole("button", { name: "Pin to sidebar" })
    expect(writes).toEqual(["DELETE /api/v1/dashboards/d1/star"])
  })

  it("is disabled until the dashboard and the pins have loaded", async () => {
    serve(false, 204)
    renderWithProviders(<PinToggle dashboardId="d1" />)

    expect(
      screen.getByRole("button", { name: "Pin to sidebar" })
    ).toBeDisabled()
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Pin to sidebar" })
      ).toBeEnabled()
    )
  })

  it("puts the toggle back and says so when the server refuses", async () => {
    serve(false, 500)
    const { user } = renderWithProviders(<PinToggle dashboardId="d1" />)

    const toggle = await screen.findByRole("button", {
      name: "Pin to sidebar",
    })
    await waitFor(() => expect(toggle).toBeEnabled())
    await user.click(toggle)

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith("Could not pin the dashboard")
    )
    expect(
      screen.getByRole("button", { name: "Pin to sidebar" })
    ).toHaveAttribute("aria-pressed", "false")
  })
})
