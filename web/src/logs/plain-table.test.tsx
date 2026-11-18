import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"
import type { WireFrame } from "@/dashboards/queries"
import { cellText, hasTimeField, PlainTable } from "./plain-table"

const untimed: WireFrame = {
  ref: "A",
  name: "",
  labels: {},
  fields: [
    { name: "service", type: "string", values: ["api", "web"] },
    { name: "errors", type: "number", values: [4, 9] },
  ],
}

describe("hasTimeField", () => {
  it("is false for an aggregate and true once a time field is present", () => {
    expect(hasTimeField([untimed])).toBe(false)
    expect(
      hasTimeField([
        {
          ...untimed,
          fields: [{ name: "Timestamp", type: "time", values: [1] }],
        },
      ])
    ).toBe(true)
  })
})

describe("cellText", () => {
  it("renders empty, scalar and object values", () => {
    expect(cellText(null)).toBe("")
    expect(cellText(7)).toBe("7")
    expect(cellText({ a: 1 })).toBe('{"a":1}')
  })
})

describe("PlainTable", () => {
  it("shows one row per value with the field names as headers", () => {
    render(<PlainTable frame={untimed} />)
    expect(screen.getByText("service")).toBeTruthy()
    expect(screen.getByText("errors")).toBeTruthy()
    expect(screen.getByText("api")).toBeTruthy()
    expect(screen.getByText("9")).toBeTruthy()
  })
})
