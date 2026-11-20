import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"
import type { PanelDataResponse, WireFrame } from "@/dashboards/queries"
import { cellText, isPlainResult, PlainTable } from "./plain-table"

const untimed: WireFrame = {
  ref: "A",
  name: "",
  labels: {},
  fields: [
    { name: "service", type: "string", values: ["api", "web"] },
    { name: "errors", type: "number", values: [4, 9] },
  ],
}

function response(
  frames: WireFrame[],
  page?: { plain?: boolean }
): PanelDataResponse {
  return { frames, meta: { page } } as unknown as PanelDataResponse
}

describe("isPlainResult", () => {
  it("is true for an aggregate with no time field and false once one is present", () => {
    expect(isPlainResult(response([untimed]))).toBe(true)
    expect(
      isPlainResult(
        response([
          {
            ...untimed,
            fields: [{ name: "Timestamp", type: "time", values: [1] }],
          },
        ])
      )
    ).toBe(false)
  })

  it("follows the server when a result has a time field but ran as a plain table", () => {
    const withMinute = {
      ...untimed,
      fields: [{ name: "minute", type: "time" as const, values: [1] }],
    }
    expect(isPlainResult(response([withMinute], { plain: true }))).toBe(true)
    expect(isPlainResult(response([withMinute], { plain: false }))).toBe(false)
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

  it("takes another note and renders the cells it is given", () => {
    render(
      <PlainTable
        frame={untimed}
        note="Each row is a service."
        renderCell={(field, value) =>
          field === "service" ? <b>{`service ${String(value)}`}</b> : null
        }
      />
    )
    expect(screen.getByText("Each row is a service.")).toBeTruthy()
    expect(screen.getByText("service api")).toBeTruthy()
    expect(screen.getByText("9")).toBeTruthy()
  })
})
