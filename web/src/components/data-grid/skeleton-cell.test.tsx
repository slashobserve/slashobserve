import { render } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { dataCell, skeletonRows, type SkeletonRow } from "./skeleton-cell"

type Row = { kind: "item"; id: string; name: string } | SkeletonRow

describe("skeletonRows", () => {
  it("makes the asked count of rows with distinct ids", () => {
    const rows = skeletonRows(3)
    expect(rows).toHaveLength(3)
    expect(new Set(rows.map((row) => row.id)).size).toBe(3)
    expect(rows.every((row) => row.kind === "skeleton")).toBe(true)
  })
})

describe("dataCell", () => {
  const cell = dataCell<Row, "item">("item", "h-4 w-8", (row) => row.name)

  it("renders the data of its kind", () => {
    const { container } = render(
      <>{cell({ kind: "item", id: "a", name: "A" })}</>
    )
    expect(container.textContent).toBe("A")
  })

  it("renders a skeleton of the given size for a skeleton row", () => {
    const { container } = render(<>{cell({ kind: "skeleton", id: "s" })}</>)
    expect(container.querySelector("[data-slot=skeleton]")).toHaveClass(
      "h-4",
      "w-8"
    )
  })
})
