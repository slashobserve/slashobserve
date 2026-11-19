import { screen, within } from "@testing-library/react"
import { afterEach, describe, expect, it } from "vitest"

import { usePreferences } from "@/lib/preferences"
import { renderWithProviders } from "@/test/render"

import { AppearanceSection } from "./appearance-card"

const storageKey = "slashobserve-theme"

describe("AppearanceSection", () => {
  afterEach(() => {
    usePreferences.setState({ timeZone: "browser", weekStart: "monday" })
    localStorage.clear()
  })

  it("offers the three theme choices under a Theme label", () => {
    renderWithProviders(<AppearanceSection />)
    const group = screen.getByRole("radiogroup", { name: "Theme" })
    expect(within(group).getAllByRole("radio")).toHaveLength(3)
    expect(group).toContainElement(screen.getByRole("radio", { name: "Light" }))
    expect(group).toContainElement(screen.getByRole("radio", { name: "Dark" }))
    expect(group).toContainElement(
      screen.getByRole("radio", { name: "Match the system" })
    )
  })

  it.each([
    { name: "Dark", dark: true, stored: "dark" },
    { name: "Light", dark: false, stored: "light" },
    { name: "Match the system", dark: false, stored: null },
  ])(
    "applies and stores the preference when $name is clicked",
    async ({ name, dark, stored }) => {
      const { user } = renderWithProviders(<AppearanceSection />)
      await user.click(screen.getByRole("radio", { name }))
      expect(screen.getByRole("radio", { name })).toBeChecked()
      expect(document.documentElement.classList.contains("dark")).toBe(dark)
      expect(localStorage.getItem(storageKey)).toBe(stored)
    }
  )

  it("stores the week start when a day is chosen", async () => {
    const { user } = renderWithProviders(<AppearanceSection />)
    const group = screen.getByRole("radiogroup", { name: "Week starts on" })
    expect(group).toContainElement(
      screen.getByRole("radio", { name: "Monday" })
    )
    expect(screen.getByRole("radio", { name: "Monday" })).toBeChecked()
    await user.click(screen.getByRole("radio", { name: "Sunday" }))
    expect(screen.getByRole("radio", { name: "Sunday" })).toBeChecked()
    expect(localStorage.getItem("slashobserve-week-start")).toBe("sunday")
  })

  it("stores the time zone when one is picked", async () => {
    const { user } = renderWithProviders(<AppearanceSection />)
    const box = screen.getByRole("combobox", { name: "Time zone" })
    expect(box).toHaveValue("Browser time")
    await user.click(box)
    await user.click(await screen.findByRole("option", { name: "UTC" }))
    expect(localStorage.getItem("slashobserve-time-zone")).toBe("utc")
    expect(usePreferences.getState().timeZone).toBe("utc")
  })
})
