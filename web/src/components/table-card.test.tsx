import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { Cell, HeaderCell, TableCard } from "./table-card"

const stackedRow = "narrow:[&_tbody_tr]:flex"

function Datasources({ stackOnNarrow }: { stackOnNarrow?: boolean }) {
  return (
    <TableCard label="Datasources" minWidth={640} stackOnNarrow={stackOnNarrow}>
      <thead>
        <tr>
          <HeaderCell>Name</HeaderCell>
          <HeaderCell>Kind</HeaderCell>
        </tr>
      </thead>
      <tbody>
        <tr>
          <Cell>prom</Cell>
          <Cell data-cell="action">Edit</Cell>
        </tr>
      </tbody>
    </TableCard>
  )
}

describe("TableCard", () => {
  it("keeps its plain table and inline min width without stackOnNarrow", () => {
    render(<Datasources />)
    const table = screen.getByRole("table")
    expect(table.className).toBe("w-full border-collapse text-[13px]")
    expect(table.style.minWidth).toBe("640px")
    expect(screen.getByRole("region", { name: "Datasources" })).toBe(
      table.parentElement
    )
  })

  it("stacks rows on narrow screens and moves the min width to a variable", () => {
    render(<Datasources stackOnNarrow />)
    const table = screen.getByRole("table")
    expect(table.classList.contains(stackedRow)).toBe(true)
    expect(table).toHaveClass("min-w-(--table-min)", "narrow:min-w-0")
    expect(table.style.minWidth).toBe("")
    expect(table.style.getPropertyValue("--table-min")).toBe("640px")
    expect(
      screen.getByRole("region", { name: "Datasources" })
    ).toBeInTheDocument()
  })

  it("passes a string min width through unchanged", () => {
    render(
      <TableCard label="Health" minWidth="40rem" stackOnNarrow>
        <tbody />
      </TableCard>
    )
    expect(
      screen.getByRole("table").style.getPropertyValue("--table-min")
    ).toBe("40rem")
  })
})
