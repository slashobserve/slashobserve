import { fireEvent, render, screen, within } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import type { LegendItem } from "./legend"
import { LegendTable } from "./legend-table"

const items: LegendItem[] = ["checkout-0", "checkout-1"].map((key, index) => ({
  key,
  label: key,
  color: `var(--series-${index + 1})`,
  line: "solid",
  style: index === 0 ? "bars" : "line",
}))

const base = {
  items,
  reducers: ["mean", "max"],
  cells: [
    ["1.20", "10"],
    ["300.50", "9"],
  ],
  hidden: new Set<string>(),
  placement: "bottom" as const,
  available: 400,
  width: 0,
  onToggle: () => undefined,
}

async function legend() {
  const group = screen.getByRole("group", { name: "Series" })
  await within(group).findAllByRole("gridcell")
  return group
}

describe("LegendTable", () => {
  it("draws a header per reducer and a row per series with its values", async () => {
    render(<LegendTable {...base} />)
    const group = await legend()
    expect(
      within(group)
        .getAllByRole("columnheader")
        .map((header) => header.textContent)
    ).toEqual(["Series", "Mean", "Max"])
    const rows = within(group).getAllByRole("button")
    expect(rows.map((row) => row.textContent)).toEqual([
      "checkout-0",
      "checkout-1",
    ])
    expect(rows.map((row) => row.dataset.style)).toEqual(["bars", "line"])
    expect(within(group).getByRole("gridcell", { name: "300.50" })).toBeTruthy()
  })

  it("marks the sorted column", async () => {
    const { rerender } = render(<LegendTable {...base} sortBy="max" sortDesc />)
    const group = await legend()
    const header = (name: string) =>
      within(group).getByRole("columnheader", { name })
    expect(header("Max")).toHaveAttribute("aria-sort", "descending")
    expect(header("Mean")).not.toHaveAttribute("aria-sort")
    rerender(<LegendTable {...base} sortBy="name" />)
    expect(header("Series")).toHaveAttribute("aria-sort", "ascending")
  })

  it("uses the headers it is given", async () => {
    render(<LegendTable {...base} headers={["Average", "Peak"]} />)
    const group = await legend()
    expect(
      within(group).getByRole("columnheader", { name: "Peak" })
    ).toBeTruthy()
  })

  it("toggles a series, dims a hidden one and reports the hovered one", async () => {
    const onToggle = vi.fn()
    const onHover = vi.fn()
    render(
      <LegendTable
        {...base}
        hidden={new Set(["checkout-1"])}
        onToggle={onToggle}
        onHover={onHover}
      />
    )
    const group = await legend()
    const second = within(group).getByRole("button", { name: "checkout-1" })
    expect(second).toHaveAttribute("aria-pressed", "false")
    expect(second).toHaveClass("opacity-50")
    expect(within(group).getByText("9")).toHaveClass("opacity-50")
    expect(within(group).getByText("10")).not.toHaveClass("opacity-50")
    fireEvent.click(second)
    expect(onToggle).toHaveBeenCalledWith("checkout-1")
    fireEvent.pointerEnter(second)
    expect(onHover).toHaveBeenLastCalledWith("checkout-1")
    fireEvent.pointerLeave(second)
    expect(onHover).toHaveBeenLastCalledWith(null)
  })

  it("lists how many series it left out", async () => {
    render(<LegendTable {...base} more={19} />)
    const group = await legend()
    expect(await within(group).findByText("+19 more")).toBeTruthy()
  })

  it("grows with its rows when they fit, and scrolls under a count when they do not", async () => {
    const { rerender } = render(<LegendTable {...base} />)
    const group = await legend()
    expect(group.style.height).toBe(`${3 * 20 + 6}px`)
    expect(screen.queryByText(/series, scroll for all/)).toBeNull()
    const many = Array.from({ length: 12 }, (_, index) => ({
      ...items[0],
      key: `pod-${index}`,
      label: `pod-${index}`,
    }))
    rerender(
      <LegendTable
        {...base}
        items={many}
        cells={many.map(() => ["1", "2"])}
        available={120}
      />
    )
    expect(group.style.height).toBe(`${(1.25 * 5 + 0.375) * 16}px`)
    expect(screen.getByText("12 series, scroll for all")).toBeTruthy()
  })

  it("takes its width on the right and never grows past its box", async () => {
    render(<LegendTable {...base} placement="right" width={240} />)
    const group = await legend()
    expect(group.style.width).toBe("240px")
    expect(group.style.maxHeight).toBe("100%")
  })
})
