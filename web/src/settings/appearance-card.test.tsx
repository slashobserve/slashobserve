import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AppearanceSection } from "./appearance-card"

const storageKey = "slashobserve-theme"

describe("AppearanceSection", () => {
  it("offers the three theme choices under a Theme label", () => {
    renderWithProviders(<AppearanceSection />)
    const group = screen.getByRole("radiogroup", { name: "Theme" })
    expect(screen.getAllByRole("radio")).toHaveLength(3)
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
})
