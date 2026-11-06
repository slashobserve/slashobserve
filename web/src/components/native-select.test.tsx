import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { NativeSelect } from "./native-select"

function Kind(props: Parameters<typeof NativeSelect>[0]) {
  return (
    <label>
      Kind
      <NativeSelect defaultValue="all" {...props}>
        <option value="all">All kinds</option>
        <option value="prometheus">Prometheus</option>
      </NativeSelect>
    </label>
  )
}

describe("NativeSelect", () => {
  it("is found by its label and reports the chosen value", async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Kind onChange={(event) => onChange(event.target.value)} />)
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Kind" }),
      "prometheus"
    )
    expect(onChange).toHaveBeenCalledWith("prometheus")
  })

  it("is h-8 by default and h-7 at size sm", () => {
    const { rerender } = render(<Kind />)
    expect(screen.getByRole("combobox", { name: "Kind" })).toHaveClass("h-8")
    rerender(<Kind size="sm" />)
    const select = screen.getByRole("combobox", { name: "Kind" })
    expect(select).toHaveClass("h-7")
    expect(select).not.toHaveClass("h-8")
  })

  it("keeps the base text size on narrow screens so iOS does not zoom", () => {
    render(<Kind size="sm" />)
    expect(screen.getByRole("combobox", { name: "Kind" })).toHaveClass(
      "text-xs",
      "narrow:text-base"
    )
  })

  it("puts wrapperClassName on the wrapper and className on the select", () => {
    render(<Kind wrapperClassName="flex-1" className="w-full" />)
    const select = screen.getByRole("combobox", { name: "Kind" })
    expect(select).toHaveClass("w-full")
    expect(select).not.toHaveClass("flex-1")
    expect(select.parentElement).toHaveClass("relative", "flex-1")
    expect(select.parentElement).not.toHaveClass("w-full")
  })

  it("passes disabled through to the select", () => {
    render(<Kind disabled />)
    expect(screen.getByRole("combobox", { name: "Kind" })).toBeDisabled()
  })
})
