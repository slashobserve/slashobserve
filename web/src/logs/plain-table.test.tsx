import { render, screen, waitFor } from "@testing-library/react"
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
  it("shows one row per value with the field names as headers", async () => {
    render(<PlainTable frame={untimed} />)
    const grid = await screen.findByRole("grid")
    await waitFor(() =>
      expect(grid).toHaveAttribute("aria-label", "Query results")
    )
    expect(
      screen
        .getAllByRole("columnheader")
        .map((header) => header.textContent?.trim())
    ).toEqual(["service", "errors"])
    expect(await screen.findByRole("gridcell", { name: "api" })).toBeTruthy()
    expect(screen.getByRole("gridcell", { name: "9" })).toBeTruthy()
    expect(screen.getByRole("gridcell", { name: "web" })).toBeTruthy()
  })

  it("takes another note and renders the cells it is given", async () => {
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
    expect(await screen.findByText("service api")).toBeTruthy()
    expect(screen.getByRole("gridcell", { name: "9" })).toBeTruthy()
  })

  it("leaves a missing value empty", async () => {
    const ragged: WireFrame = {
      ...untimed,
      fields: [
        { name: "service", type: "string", values: ["api", "web"] },
        { name: "errors", type: "number", values: [4] },
      ],
    }
    const { container } = render(<PlainTable frame={ragged} />)
    await screen.findByRole("gridcell", { name: "web" })
    const cell = container.querySelector('[row-id="1"] [col-id="errors"]')
    expect(cell).not.toBeNull()
    expect(cell?.textContent).toBe("")
  })
})
