import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { NotFoundPage } from "./not-found-page"

describe("NotFoundPage", () => {
  it("names the address and offers a way back, in the shell", async () => {
    renderWithProviders(<NotFoundPage />, {
      route: { path: "$", url: "/nope" },
    })
    expect(
      await screen.findByRole("heading", { name: "Page not found" })
    ).toBeInTheDocument()
    expect(screen.getByText("/nope")).toBeInTheDocument()
    expect(screen.getByRole("link", { name: "Back to home" })).toHaveAttribute(
      "href",
      "/"
    )
    expect(document.title).toBe("Page not found - SlashObserve")
  })
})
