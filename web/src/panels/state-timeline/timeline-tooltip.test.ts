import { describe, expect, it } from "vitest"

import type { TimelineRow } from "./timeline-model"
import { maxTooltipEntries, tooltipEntries } from "./timeline-tooltip"

function row(name: string, value: number | null, text = String(value)) {
  return {
    key: name,
    name,
    opacity: 1,
    border: 0,
    segments: [{ from: 0, to: 100, text, color: "red", value }],
  } satisfies TimelineRow
}

const rows = [row("a", 2), row("b", 0), row("c", 5), row("d", null, "up")]
const names = (entries: { name: string }[]) => entries.map((e) => e.name)

describe("tooltipEntries", () => {
  it("lists every row that has a state at the time, in series order for none", () => {
    const { entries, total } = tooltipEntries(rows, 50, {
      sort: "none",
      hideZeros: false,
    })
    expect(names(entries)).toEqual(["a", "b", "c", "d"])
    expect(total).toBe(4)
  })

  it("sorts rows with a number and puts text states after them", () => {
    expect(
      names(
        tooltipEntries(rows, 50, { sort: "desc", hideZeros: false }).entries
      )
    ).toEqual(["c", "a", "b", "d"])
    expect(
      names(tooltipEntries(rows, 50, { sort: "asc", hideZeros: false }).entries)
    ).toEqual(["b", "a", "c", "d"])
  })

  it("hides rows whose value is zero and never a text state", () => {
    const { entries, total } = tooltipEntries(rows, 50, {
      sort: "none",
      hideZeros: true,
    })
    expect(names(entries)).toEqual(["a", "c", "d"])
    expect(total).toBe(3)
  })

  it("skips rows with no state at the time", () => {
    expect(
      tooltipEntries(rows, 150, { sort: "none", hideZeros: false }).entries
    ).toEqual([])
  })

  it("lists at most ten rows and counts all of them", () => {
    const many = Array.from({ length: 14 }, (_, index) =>
      row(`r${index}`, index)
    )
    const { entries, total } = tooltipEntries(many, 1, {
      sort: "none",
      hideZeros: false,
    })
    expect(entries).toHaveLength(maxTooltipEntries)
    expect(total).toBe(14)
  })
})
