import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { PromptBubble } from "./prompt-bubble"

describe("PromptBubble", () => {
  it("shows a short prompt whole with no toggle", () => {
    render(<PromptBubble prompt="checkout health" />)
    expect(screen.getByText("checkout health")).not.toHaveClass("line-clamp-6")
    expect(
      screen.queryByRole("button", { name: "Show the whole message" })
    ).not.toBeInTheDocument()
  })

  it("copies the whole message even when it is folded", async () => {
    const user = userEvent.setup()
    const writeText = vi.spyOn(navigator.clipboard, "writeText")
    render(<PromptBubble prompt={"x".repeat(3900)} />)
    await user.click(screen.getByRole("button", { name: "Copy message" }))
    expect(writeText).toHaveBeenCalledWith("x".repeat(3900))
  })

  it("collapses a long prompt and expands it on request", async () => {
    const user = userEvent.setup()
    render(<PromptBubble prompt={"x".repeat(3900)} />)
    const body = screen.getByText("x".repeat(3900))
    expect(body).toHaveClass("line-clamp-6")
    const toggle = screen.getByRole("button", {
      name: "Show the whole message",
    })
    expect(toggle).toHaveAttribute("aria-expanded", "false")
    await user.click(toggle)
    expect(body).not.toHaveClass("line-clamp-6")
    expect(screen.getByRole("button", { name: "Show less" })).toHaveAttribute(
      "aria-expanded",
      "true"
    )
  })
})
