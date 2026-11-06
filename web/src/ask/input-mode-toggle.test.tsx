import { fireEvent, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import type { InputMode } from "./input-mode"
import { InputModeToggle } from "./input-mode-toggle"

const labels: Record<InputMode, string> = {
  ask: "Ask",
  builder: "Builder",
  code: "Query",
}
const two: readonly InputMode[] = ["ask", "code"]
const three: readonly InputMode[] = ["builder", "code", "ask"]

describe("InputModeToggle", () => {
  it("renders the modes it is given with the selected one checked", () => {
    render(
      <InputModeToggle
        mode="ask"
        modes={two}
        labels={labels}
        agentAvailable
        onChange={vi.fn()}
      />
    )
    expect(screen.getByRole("radiogroup", { name: "Input mode" })).toBeTruthy()
    expect(screen.getAllByRole("radio")).toHaveLength(2)
    expect(screen.getByRole("radio", { name: "Ask" })).toHaveAttribute(
      "aria-checked",
      "true"
    )
    expect(screen.getByRole("radio", { name: "Query" })).toHaveAttribute(
      "aria-checked",
      "false"
    )
  })

  it("renders three modes in the order it is given", () => {
    render(
      <InputModeToggle
        mode="builder"
        modes={three}
        labels={labels}
        agentAvailable
        onChange={vi.fn()}
      />
    )
    expect(
      screen.getAllByRole("radio").map((radio) => radio.textContent)
    ).toEqual(["Builder", "Query", "Ask"])
  })

  it("calls onChange with code when Query is clicked", async () => {
    const onChange = vi.fn()
    render(
      <InputModeToggle
        mode="ask"
        modes={two}
        labels={labels}
        agentAvailable
        onChange={onChange}
      />
    )
    await userEvent.click(screen.getByRole("radio", { name: "Query" }))
    expect(onChange).toHaveBeenCalledWith("code")
  })

  it("disables Ask without a model and says why", async () => {
    const onChange = vi.fn()
    render(
      <InputModeToggle
        mode="code"
        modes={two}
        labels={labels}
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

  it.each([
    ["ask", "ArrowRight", "code"],
    ["code", "ArrowRight", "ask"],
    ["ask", "ArrowLeft", "code"],
  ] as const)("moves from %s with %s to %s", (from, key, target) => {
    const onChange = vi.fn()
    render(
      <InputModeToggle
        mode={from}
        modes={two}
        labels={labels}
        agentAvailable
        onChange={onChange}
      />
    )
    fireEvent.keyDown(screen.getByRole("radio", { name: labels[from] }), {
      key,
    })
    expect(onChange).toHaveBeenCalledWith(target)
  })

  it("walks three modes with the arrow keys", () => {
    const onChange = vi.fn()
    render(
      <InputModeToggle
        mode="builder"
        modes={three}
        labels={labels}
        agentAvailable
        onChange={onChange}
      />
    )
    fireEvent.keyDown(screen.getByRole("radio", { name: "Builder" }), {
      key: "ArrowLeft",
    })
    expect(onChange).toHaveBeenCalledWith("ask")
    fireEvent.keyDown(screen.getByRole("radio", { name: "Builder" }), {
      key: "ArrowRight",
    })
    expect(onChange).toHaveBeenCalledWith("code")
  })
})
