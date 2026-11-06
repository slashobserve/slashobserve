import { fireEvent, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { InputModeToggle } from "./input-mode-toggle"

describe("InputModeToggle", () => {
  it("renders two radios with the selected one checked", () => {
    render(<InputModeToggle mode="ask" agentAvailable onChange={vi.fn()} />)
    expect(screen.getByRole("radiogroup", { name: "Input mode" })).toBeTruthy()
    expect(screen.getByRole("radio", { name: "Ask" })).toHaveAttribute(
      "aria-checked",
      "true"
    )
    expect(screen.getByRole("radio", { name: "Query" })).toHaveAttribute(
      "aria-checked",
      "false"
    )
  })

  it("calls onChange with query when Query is clicked", async () => {
    const onChange = vi.fn()
    render(<InputModeToggle mode="ask" agentAvailable onChange={onChange} />)
    await userEvent.click(screen.getByRole("radio", { name: "Query" }))
    expect(onChange).toHaveBeenCalledWith("query")
  })

  it("disables Ask without a model and says why", async () => {
    const onChange = vi.fn()
    render(
      <InputModeToggle
        mode="query"
        agentAvailable={false}
        onChange={onChange}
      />
    )
    const ask = screen.getByRole("radio", { name: "Ask" })
    expect(ask).toHaveAttribute("aria-disabled", "true")
    expect(ask).toHaveAccessibleDescription(
      "Add a model key to ask in plain language"
    )
    await userEvent.click(ask)
    expect(onChange).not.toHaveBeenCalled()
  })

  it("moves to Query with ArrowRight on Ask", () => {
    const onChange = vi.fn()
    render(<InputModeToggle mode="ask" agentAvailable onChange={onChange} />)
    fireEvent.keyDown(screen.getByRole("radio", { name: "Ask" }), {
      key: "ArrowRight",
    })
    expect(onChange).toHaveBeenCalledWith("query")
  })
})
