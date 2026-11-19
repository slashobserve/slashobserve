import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { RowHeader } from "./row-header"

describe("RowHeader", () => {
  it("names the toggle with the row title and states the fold in aria-expanded", () => {
    const { rerender } = render(
      <RowHeader
        rowId="r1"
        title="Traffic"
        collapsed={false}
        childCount={2}
        onToggle={() => {}}
      />
    )
    const open = screen.getByRole("button", { name: "Traffic" })
    expect(open).toHaveAttribute("aria-expanded", "true")
    rerender(
      <RowHeader
        rowId="r1"
        title="Traffic"
        collapsed
        childCount={2}
        onToggle={() => {}}
      />
    )
    expect(screen.getByRole("button", { name: "Traffic" })).toHaveAttribute(
      "aria-expanded",
      "false"
    )
  })
})
