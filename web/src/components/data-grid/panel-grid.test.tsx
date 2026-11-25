import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import { createRef } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { drawnColumns, drawnRows } from "./grid-dom"
import {
  PanelGrid,
  footerRowId,
  type PanelGridColumn,
  type PanelGridHandle,
} from "./panel-grid"

type Row = { id: string; name: string; count: number }

const rows: Row[] = [
  { id: "a", name: "checkout", count: 3 },
  { id: "b", name: "search", count: 5 },
]

const columns: PanelGridColumn<Row>[] = [
  {
    id: "name",
    label: "Name",
    cell: (row) => <span data-value="">{row.name}</span>,
    footer: (index) => <span data-footer-label="">{`Total ${index}`}</span>,
  },
  {
    id: "count",
    label: "Count",
    cell: (row) => String(row.count),
    footer: () => "8",
  },
]

const gridErrors = vi.spyOn(console, "error")
const gridWarnings = vi.spyOn(console, "warn")

afterEach(() => {
  const reports = [...gridErrors.mock.calls, ...gridWarnings.mock.calls]
    .map((call) => String(call[0]))
    .filter((message) => message.includes("AG Grid"))
  gridErrors.mockClear()
  gridWarnings.mockClear()
  expect(reports).toEqual([])
})

const base = {
  label: "Requests",
  rowId: (row: Row) => row.id,
  rowHeight: 25,
  headerHeight: 32,
}

describe("PanelGrid", () => {
  it("draws a labelled grid with the columns' own headers and cells", async () => {
    const withHeader = columns.map((column) =>
      column.id === "count"
        ? { ...column, header: () => <b data-own-header="">Count</b> }
        : column
    )
    const { container } = render(
      <PanelGrid {...base} rows={rows} columns={withHeader} />
    )
    const grid = await screen.findByRole("grid")
    await waitFor(() => expect(grid).toHaveAttribute("aria-label", "Requests"))
    expect(
      screen
        .getAllByRole("columnheader")
        .map((header) => header.textContent?.trim())
    ).toEqual(["Name", "Count"])
    expect(container.querySelector("[data-own-header]")).not.toBeNull()
    expect(screen.getByRole("gridcell", { name: "search" })).toBeInTheDocument()
    expect([...drawnColumns(container).keys()]).toEqual(["name", "count"])
    expect([...drawnRows(container).keys()]).toEqual(["a", "b"])
  })

  it("sets aria-sort from the column and sorts on Enter", async () => {
    const onSort = vi.fn()
    const sorted = columns.map((column) =>
      column.id === "count"
        ? { ...column, sort: "descending" as const, onSort }
        : column
    )
    render(<PanelGrid {...base} rows={rows} columns={sorted} />)
    const count = await screen.findByRole("columnheader", { name: "Count" })
    expect(count).toHaveAttribute("aria-sort", "descending")
    expect(
      screen.getByRole("columnheader", { name: "Name" })
    ).not.toHaveAttribute("aria-sort")
    fireEvent.keyDown(count, { key: "Enter" })
    expect(onSort).toHaveBeenCalledTimes(1)
  })

  it("pins the first columns to the left", async () => {
    const pinned = columns.map((column, index) => ({
      ...column,
      width: 120,
      pinned: index === 0,
    }))
    render(<PanelGrid {...base} rows={rows} columns={pinned} />)
    await screen.findByRole("gridcell", { name: "checkout" })
    const header = screen.getByRole("columnheader", { name: "Name" })
    expect(header.closest(".ag-grid-pinned-left-cells")).not.toBeNull()
    const count = screen.getByRole("columnheader", { name: "Count" })
    expect(count.closest(".ag-grid-pinned-left-cells")).toBeNull()
    const cell = screen.getByRole("gridcell", { name: "checkout" })
    expect(cell.closest(".ag-grid-pinned-left-cells")).not.toBeNull()
  })

  it("pins footer rows to the bottom with each column's footer", async () => {
    const { container } = render(
      <PanelGrid {...base} rows={rows} columns={columns} footerRows={2} />
    )
    await screen.findByRole("gridcell", { name: "checkout" })
    const footers = [
      ...container.querySelectorAll(".ag-grid-pinned-bottom-rows [role=row]"),
    ].map((row) => row.getAttribute("row-id"))
    expect(footers).toEqual([footerRowId(0), footerRowId(1)])
    const labels = [...container.querySelectorAll("[data-footer-label]")].map(
      (label) => label.textContent
    )
    expect(labels).toEqual(["Total 0", "Total 1"])
  })

  it("draws an opened row's detail below it, across the grid", async () => {
    const props = {
      ...base,
      rows,
      columns,
      details: {
        open: new Set(["a"]),
        render: (row: Row) => <p>{`${row.name} detail`}</p>,
      },
    }
    const { container, rerender } = render(<PanelGrid {...props} />)
    expect(await screen.findByText("checkout detail")).toBeInTheDocument()
    const detail = container.querySelector("[data-detail-row]")
    const rowOf = (element: Element | null) =>
      element?.closest("[role=row]")?.getAttribute("row-id")
    expect(rowOf(detail)).not.toBe("a")
    expect(rowOf(detail)).toMatch(/^a/)
    expect(screen.queryByText("search detail")).not.toBeInTheDocument()
    rerender(
      <PanelGrid {...props} details={{ ...props.details, open: new Set() }} />
    )
    await waitFor(() =>
      expect(screen.queryByText("checkout detail")).not.toBeInTheDocument()
    )
  })

  it("pages the rows, reports the page and goes to the page of a revealed row", async () => {
    const many = Array.from({ length: 7 }, (_, index) => ({
      id: `r${index}`,
      name: `row ${index}`,
      count: index,
    }))
    const onPageChange = vi.fn()
    const handle = createRef<PanelGridHandle>()
    render(
      <PanelGrid
        {...base}
        ref={handle}
        rows={many}
        columns={columns}
        pageSize={3}
        onPageChange={onPageChange}
      />
    )
    await screen.findByRole("gridcell", { name: "row 0" })
    await waitFor(() =>
      expect(onPageChange).toHaveBeenLastCalledWith({ page: 0, pageCount: 3 })
    )
    expect(screen.queryByRole("gridcell", { name: "row 3" })).toBeNull()
    handle.current?.reveal("r4")
    await waitFor(() =>
      expect(onPageChange).toHaveBeenLastCalledWith({ page: 1, pageCount: 3 })
    )
    expect(
      await screen.findByRole("gridcell", { name: "row 4" })
    ).toBeInTheDocument()
    handle.current?.goToPage(2)
    expect(
      await screen.findByRole("gridcell", { name: "row 6" })
    ).toBeInTheDocument()
  })

  it("clicks a cell's one control on Enter", async () => {
    const onOpen = vi.fn()
    const withButton = columns.map((column) =>
      column.id === "name"
        ? {
            ...column,
            cell: (row: Row) => (
              <button type="button" onClick={() => onOpen(row.id)}>
                {row.name}
              </button>
            ),
          }
        : column
    )
    render(<PanelGrid {...base} rows={rows} columns={withButton} />)
    const button = await screen.findByRole("button", { name: "search" })
    const cell = button.closest("[role=gridcell]") as HTMLElement
    fireEvent.keyDown(cell, { key: "Enter" })
    expect(onOpen).toHaveBeenCalledWith("b")
  })

  it("gives rows the row height and footers their own", async () => {
    const { container } = render(
      <PanelGrid
        {...base}
        rowHeight={41}
        footerHeight={25}
        rows={rows}
        columns={columns}
        footerRows={1}
      />
    )
    await screen.findByRole("gridcell", { name: "checkout" })
    const height = (id: string) =>
      container.querySelector<HTMLElement>(`[role=row][row-id="${id}"]`)?.style
        .height
    expect(height("a")).toBe("41px")
    expect(height(footerRowId(0))).toBe("25px")
  })
})
