import { screen, waitFor } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { PhoneBar } from "./phone-bar"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({
    principal: {
      id: "u1",
      name: "Ana",
      email: "ana@example.com",
      role: "admin",
    },
  }),
}))

describe("PhoneBar menu", () => {
  it("opens a modal dialog named Menu that Escape closes and returns focus from", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ items: [], next_cursor: null }), {
            status: 200,
            headers: { "content-type": "application/json" },
          })
      )
    )
    const { user } = renderWithProviders(<PhoneBar />, {
      route: { path: "/", url: "/" },
    })
    const menu = await screen.findByRole("button", { name: "Menu" })
    await user.click(menu)
    const dialog = await screen.findByRole("dialog", { name: "Menu" })
    expect(dialog).toHaveAttribute("aria-modal", "true")
    await user.keyboard("{Escape}")
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Menu" })).toBeNull()
    )
    expect(menu).toHaveFocus()
    vi.unstubAllGlobals()
  })
})
