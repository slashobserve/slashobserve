import { createRef } from "react"
import { fireEvent, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { PreparedFrame } from "@/panels/transforms/names"
import type { DataPanel } from "@/spec"
import { renderWithProviders } from "@/test/render"

import { downloadCsv } from "./csv"
import { DataTab } from "./data-tab"
import type { InspectTarget } from "./inspect-pane"

vi.mock("./csv", async (load) => ({
  ...(await load<typeof import("./csv")>()),
  downloadCsv: vi.fn(),
}))

const panel = {
  id: "p1",
  type: "timeseries",
  title: "Requests",
  gridPos: { x: 0, y: 0, w: 12, h: 6 },
  datasource: "prom",
  queries: [{ ref: "A", query: "up" }],
  unit: "percent",
} as DataPanel

const start = Date.parse("2026-11-06T10:00:00Z")

function frameOf(count: number): PreparedFrame {
  return {
    ref: "A",
    fields: [
      {
        name: "Time",
        type: "time",
        values: Array.from({ length: count }, (_, index) => start + index),
      },
      {
        name: "Value",
        type: "number",
        values: Array.from({ length: count }, (_, index) => index),
        seriesName: "cpu",
      },
    ],
  }
}

function targetWith(frames: PreparedFrame[]): InspectTarget {
  return {
    panel,
    title: "Requests",
    data: undefined,
    frames,
    queryKey: ["panel"],
    selections: {},
    body: createRef(),
    refetch: () => {},
  }
}

afterEach(() => vi.mocked(downloadCsv).mockClear())

describe("DataTab", () => {
  it("waits for the panel's frames", () => {
    renderWithProviders(<DataTab target={null} title="Requests" zone="UTC" />)
    expect(screen.getByText(/Waiting for the panel's data/)).toBeTruthy()
  })

  it("draws the frames as a labelled grid with formatted values", async () => {
    const frames = [frameOf(2)]
    renderWithProviders(
      <DataTab target={targetWith(frames)} title="Requests" zone="UTC" />
    )
    const grid = await screen.findByRole("grid")
    await waitFor(() =>
      expect(grid).toHaveAttribute("aria-label", "Requests data")
    )
    expect(
      screen
        .getAllByRole("columnheader")
        .map((header) => header.textContent?.trim())
    ).toEqual(["Time", "cpu"])
    expect(
      await screen.findByRole("gridcell", { name: "1%" })
    ).toBeInTheDocument()
    expect(
      screen.getByText("2 rows, after the panel's transforms.")
    ).toBeTruthy()
  })

  it("shows raw values when Formatted data is off", async () => {
    renderWithProviders(
      <DataTab target={targetWith([frameOf(2)])} title="Requests" zone="UTC" />
    )
    await screen.findByRole("gridcell", { name: "1%" })
    await userEvent.click(
      screen.getByRole("checkbox", { name: "Formatted data" })
    )
    expect(
      await screen.findByRole("gridcell", { name: "1" })
    ).toBeInTheDocument()
  })

  it("draws the first 500 rows and says the CSV has every row", async () => {
    const { container } = renderWithProviders(
      <DataTab
        target={targetWith([frameOf(600)])}
        title="Requests"
        zone="UTC"
      />
    )
    await screen.findByRole("gridcell", { name: "0%" })
    expect(
      screen.getByText(
        "The first 500 of 600 rows, after the panel's transforms. The CSV has every row."
      )
    ).toBeTruthy()
    expect(container.querySelector('[data-dom-layout="normal"]')).not.toBeNull()
    expect(container.querySelector('[row-id="500"]')).toBeNull()
  })

  it("downloads every frame as CSV", async () => {
    const frames = [frameOf(3)]
    renderWithProviders(
      <DataTab target={targetWith(frames)} title="Requests" zone="UTC" />
    )
    fireEvent.click(screen.getByRole("button", { name: "Download CSV" }))
    expect(downloadCsv).toHaveBeenCalledWith(frames, "Requests")
  })

  it("draws no grid for frames without fields", () => {
    renderWithProviders(
      <DataTab
        target={targetWith([{ ref: "A", fields: [] }])}
        title="Requests"
        zone="UTC"
      />
    )
    expect(screen.queryByRole("grid")).toBeNull()
    expect(within(document.body).getByText(/0 rows/)).toBeTruthy()
  })
})
