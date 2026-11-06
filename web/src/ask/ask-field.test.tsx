import { useState } from "react"
import { fireEvent, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AskField } from "./ask-field"

function Field({
  onAsk = vi.fn(),
  onCancel = vi.fn(),
  pending = false,
  initial = "",
}: {
  onAsk?: (question: string) => void
  onCancel?: () => void
  pending?: boolean
  initial?: string
}) {
  const [value, setValue] = useState(initial)
  return (
    <AskField
      signal="logs"
      value={value}
      onChange={setValue}
      onAsk={onAsk}
      pending={pending}
      onCancel={onCancel}
      placeholder="Ask in plain language"
    />
  )
}

afterEach(() => localStorage.clear())

describe("AskField", () => {
  it("asks the trimmed question on Enter", async () => {
    const onAsk = vi.fn()
    render(<Field onAsk={onAsk} />)
    await userEvent.type(
      screen.getByRole("textbox", { name: "Question" }),
      "  errors from checkout {Enter}"
    )
    expect(onAsk).toHaveBeenCalledWith("errors from checkout")
  })

  it("adds a line on Shift+Enter without asking", async () => {
    const onAsk = vi.fn()
    render(<Field onAsk={onAsk} />)
    const field = screen.getByRole("textbox", { name: "Question" })
    await userEvent.type(field, "errors{Shift>}{Enter}{/Shift}slow")
    expect(onAsk).not.toHaveBeenCalled()
    expect(field).toHaveValue("errors\nslow")
  })

  it("says Writing query while pending", () => {
    render(<Field pending initial="errors" />)
    expect(screen.getByRole("button", { name: "Writing query" })).toBeTruthy()
    expect(screen.getByRole("textbox", { name: "Question" })).toBeEnabled()
  })

  it("cancels on Escape while pending", () => {
    const onCancel = vi.fn()
    render(<Field pending onCancel={onCancel} initial="errors" />)
    fireEvent.keyDown(screen.getByRole("textbox", { name: "Question" }), {
      key: "Escape",
    })
    expect(onCancel).toHaveBeenCalled()
  })

  it("fills the last recorded ask on Up in an empty field", () => {
    localStorage.setItem(
      "slashobserve-ask-history-logs",
      JSON.stringify(["newest ask", "older ask"])
    )
    render(<Field />)
    const field = screen.getByRole("textbox", { name: "Question" })
    fireEvent.keyDown(field, { key: "ArrowUp" })
    expect(field).toHaveValue("newest ask")
  })

  it("records an ask so the next Up recalls it", async () => {
    render(<Field />)
    const field = screen.getByRole("textbox", { name: "Question" })
    await userEvent.type(field, "slow checkout{Enter}")
    expect(
      JSON.parse(localStorage.getItem("slashobserve-ask-history-logs") ?? "[]")
    ).toEqual(["slow checkout"])
  })

  it("lists recent asks on focus and fills one on click", async () => {
    localStorage.setItem(
      "slashobserve-ask-history-logs",
      JSON.stringify(["newest ask", "older ask"])
    )
    render(<Field />)
    const field = screen.getByRole("textbox", { name: "Question" })
    await userEvent.click(field)
    await userEvent.click(screen.getByRole("button", { name: "older ask" }))
    expect(field).toHaveValue("older ask")
  })
})
