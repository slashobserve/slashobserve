import { useState } from "react"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import type { DraftAction } from "./draft"
import { UnitSelect } from "./unit-select"

function Harness({
  initial,
  onAction,
}: {
  initial: string | undefined
  onAction: (action: DraftAction) => void
}) {
  const [unit, setUnit] = useState(initial)
  return (
    <UnitSelect
      unit={unit}
      readOnly={false}
      dispatch={(action) => {
        onAction(action)
        if (action.type === "setUnit") setUnit(action.unit ?? undefined)
      }}
    />
  )
}

function renderUnit(unit: string | undefined) {
  const dispatch = vi.fn<(action: DraftAction) => void>()
  const user = userEvent.setup()
  render(<Harness initial={unit} onAction={dispatch} />)
  return {
    dispatch,
    user,
    input: screen.getByRole("combobox", { name: "Unit" }),
  }
}

describe("UnitSelect", () => {
  it("shows the current unit's label", () => {
    expect(renderUnit("percent").input).toHaveValue(
      "percent, 0 to 100 (percent)"
    )
  })

  it("shows None when no unit is set", () => {
    expect(renderUnit(undefined).input).toHaveValue("None")
  })

  it("filters the catalog by label and sets the chosen unit", async () => {
    const { dispatch, user, input } = renderUnit(undefined)
    await user.click(input)
    await user.clear(input)
    await user.type(input, "decibel")
    const options = await screen.findAllByRole("option")
    expect(options.map((option) => option.textContent)).toEqual([
      "decibel (dB)",
      "decibel-milliwatts (dBm)",
    ])
    await user.click(options[0])
    expect(dispatch).toHaveBeenCalledWith({ type: "setUnit", unit: "dB" })
  })

  it("clears the unit when None is chosen", async () => {
    const { dispatch, user, input } = renderUnit("percent")
    await user.click(input)
    await user.click(await screen.findByRole("option", { name: "None" }))
    expect(dispatch).toHaveBeenCalledWith({ type: "setUnit", unit: null })
  })

  it("opens the custom text field when a custom kind is chosen", async () => {
    const { dispatch, user, input } = renderUnit(undefined)
    await user.click(input)
    await user.click(
      await screen.findByRole("option", { name: "Custom suffix" })
    )
    expect(input).toHaveValue("Custom suffix")
    await user.type(screen.getByRole("textbox", { name: "Suffix" }), "req")
    expect(dispatch).toHaveBeenLastCalledWith({
      type: "setUnit",
      unit: "suffix:req",
    })
  })
})
