import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { RequestNotFound } from "./request-not-found"

function render(props: { removed: boolean; pane: boolean }) {
  return renderWithProviders(<RequestNotFound {...props} />, {
    route: { path: "/", url: "/" },
  })
}

describe("RequestNotFound", () => {
  it("says the same thing on the chat page for a chat that is not there or not yours", async () => {
    render({ removed: false, pane: false })
    expect(
      await screen.findByRole("heading", { name: "You can't open this chat" })
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        "It does not exist, or only the person who asked and workspace admins can open it."
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole("link", { name: "Back to Observe" })
    ).toHaveAttribute("href", "/ask")
  })

  it("says a deleted chat no longer exists", async () => {
    render({ removed: true, pane: false })
    expect(
      await screen.findByText("This chat no longer exists.")
    ).toBeInTheDocument()
  })

  it("keeps the same words in the pane beside a build", async () => {
    render({ removed: false, pane: true })
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "You can't open this chat. It does not exist, or only the person who asked and workspace admins can open it."
    )
    expect(screen.queryByRole("heading")).not.toBeInTheDocument()
  })
})
