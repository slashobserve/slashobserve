import { screen, waitFor } from "@testing-library/react"
import type { UserEvent } from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AccountMenu } from "./account-menu"

const signOut = vi.hoisted(() => {
  localStorage.setItem("slashobserve-theme", "light")
  return vi.fn()
})

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({
    principal: {
      id: "u1",
      name: "Ana Lima",
      email: "ana@example.com",
      role: "admin",
    },
  }),
}))

vi.mock("./use-sign-out", () => ({
  useSignOut: () => ({ signOut, pending: false }),
}))

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 200 }))
  )
  return () => vi.unstubAllGlobals()
})

async function openTheme() {
  const { user } = renderWithProviders(<AccountMenu collapsed={false} />)
  await user.click(
    screen.getByRole("button", { name: "Account: Ana Lima, ana@example.com" })
  )
  await openThemeSubmenu(user)
  return user
}

async function openThemeSubmenu(user: UserEvent) {
  const theme = await screen.findByRole("menuitem", { name: "Theme" })
  theme.focus()
  await user.keyboard("{ArrowRight}")
  await screen.findByRole("menuitemradio", { name: "Light" })
}

async function choose(user: UserEvent, name: string) {
  const radio = screen.getByRole("menuitemradio", { name })
  radio.focus()
  await user.keyboard("{Enter}")
}

describe("AccountMenu", () => {
  it("offers Theme above Sign out", async () => {
    const { user } = renderWithProviders(<AccountMenu collapsed />)
    await user.click(screen.getByRole("button", { name: /^Account: / }))
    const items = await screen.findAllByRole("menuitem")
    expect(items.map((item) => item.textContent)).toEqual(["Theme", "Sign out"])
    await user.click(screen.getByRole("menuitem", { name: "Sign out" }))
    expect(signOut).toHaveBeenCalledOnce()
  })

  it("checks the stored theme and applies the one chosen", async () => {
    const user = await openTheme()
    const radios = screen.getAllByRole("menuitemradio")
    expect(radios.map((radio) => radio.textContent)).toEqual([
      "Light",
      "Dark",
      "Match the system",
    ])
    expect(
      screen.getByRole("menuitemradio", { name: "Light" })
    ).toHaveAttribute("aria-checked", "true")

    await choose(user, "Dark")
    await waitFor(() => expect(document.documentElement).toHaveClass("dark"))
    expect(localStorage.getItem("slashobserve-theme")).toBe("dark")
  })

  it("stores Match the system", async () => {
    const user = await openTheme()
    await choose(user, "Match the system")
    await waitFor(() =>
      expect(localStorage.getItem("slashobserve-theme")).toBeNull()
    )
    await user.click(screen.getByRole("button", { name: /^Account: / }))
    await openThemeSubmenu(user)
    expect(
      await screen.findByRole("menuitemradio", { name: "Match the system" })
    ).toHaveAttribute("aria-checked", "true")
  })
})
