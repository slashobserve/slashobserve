import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { DataGrid, type DataGridColumn } from "./data-grid"
import { useNarrow } from "./use-narrow"

vi.mock("./use-narrow", () => ({ useNarrow: vi.fn(() => false) }))

type Row = { id: string; name: string; owner: string; note: string }

const rows: Row[] = [
  { id: "a", name: "Checkout", owner: "Payments", note: "" },
  { id: "b", name: "Search", owner: "Discovery", note: "slow" },
]

const columns: DataGridColumn<Row>[] = [
  { id: "name", header: "Name", sortable: true, cell: (row) => row.name },
  { id: "owner", header: "Owner", sortable: true, cell: (row) => row.owner },
  { id: "note", header: "Note", cell: (row) => row.note || null },
  {
    id: "open",
    header: "Open",
    action: true,
    cell: (row) => <a href={`#${row.id}`}>Open {row.name}</a>,
  },
]

const gridErrors = vi.spyOn(console, "error")
const gridWarnings = vi.spyOn(console, "warn")

afterEach(() => {
  vi.mocked(useNarrow).mockReturnValue(false)
  const reports = [...gridErrors.mock.calls, ...gridWarnings.mock.calls]
    .map((call) => String(call[0]))
    .filter((message) => message.includes("AG Grid"))
  gridErrors.mockClear()
  gridWarnings.mockClear()
  expect(reports).toEqual([])
})

describe("DataGrid", () => {
  it("draws a labelled grid with a header and a cell per column", async () => {
    render(
      <DataGrid
        label="Services"
        rows={rows}
        columns={columns}
        rowId={(row) => row.id}
      />
    )
    const grid = await screen.findByRole("grid")
    await waitFor(() => expect(grid).toHaveAttribute("aria-label", "Services"))
    expect(
      screen
        .getAllByRole("columnheader")
        .map((header) => header.textContent?.trim())
    ).toEqual(["Name", "Owner", "Note", "Open"])
    expect(
      screen.getByRole("gridcell", { name: "Checkout" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("link", { name: "Open Search" })
    ).toBeInTheDocument()
  })

  it("marks the sorted column and asks for the next sort on a header click", async () => {
    const onSort = vi.fn()
    render(
      <DataGrid
        label="Services"
        rows={rows}
        columns={columns}
        rowId={(row) => row.id}
        sort={{ key: "owner", direction: "descending" }}
        onSort={onSort}
      />
    )
    const owner = await screen.findByRole("columnheader", { name: "Owner" })
    expect(owner).toHaveAttribute("aria-sort", "descending")
    expect(screen.getByRole("columnheader", { name: "Name" })).toHaveAttribute(
      "aria-sort",
      "none"
    )
    expect(
      screen.getByRole("columnheader", { name: "Note" })
    ).not.toHaveAttribute("aria-sort")
    fireEvent.click(within(owner).getByRole("button", { name: "Owner" }))
    expect(onSort).toHaveBeenLastCalledWith("owner")
    fireEvent.keyDown(screen.getByRole("columnheader", { name: "Name" }), {
      key: "Enter",
    })
    expect(onSort).toHaveBeenLastCalledWith("name")
  })

  it("moves aria-sort when the sort changes", async () => {
    const props = {
      label: "Services",
      rows,
      columns,
      rowId: (row: Row) => row.id,
      onSort: () => {},
    }
    const { rerender } = render(
      <DataGrid {...props} sort={{ key: "name", direction: "ascending" }} />
    )
    const name = await screen.findByRole("columnheader", { name: "Name" })
    expect(name).toHaveAttribute("aria-sort", "ascending")
    rerender(
      <DataGrid {...props} sort={{ key: "owner", direction: "ascending" }} />
    )
    await waitFor(() => expect(name).toHaveAttribute("aria-sort", "none"))
    expect(screen.getByRole("columnheader", { name: "Owner" })).toHaveAttribute(
      "aria-sort",
      "ascending"
    )
  })

  it("redraws a cell when its column changes", async () => {
    const { rerender } = render(
      <DataGrid
        label="Services"
        rows={rows}
        columns={columns}
        rowId={(row) => row.id}
      />
    )
    await screen.findByRole("gridcell", { name: "Checkout" })
    const renamed = columns.map((column) =>
      column.id === "name"
        ? { ...column, cell: (row: Row) => `${row.name} service` }
        : column
    )
    rerender(
      <DataGrid
        label="Services"
        rows={rows}
        columns={renamed}
        rowId={(row) => row.id}
      />
    )
    expect(
      await screen.findByRole("gridcell", { name: "Checkout service" })
    ).toBeInTheDocument()
  })

  it("follows the link of a cell on Enter", async () => {
    render(
      <DataGrid
        label="Services"
        rows={rows}
        columns={columns}
        rowId={(row) => row.id}
      />
    )
    const link = await screen.findByRole("link", { name: "Open Checkout" })
    const clicked = vi.fn((event: MouseEvent) => event.preventDefault())
    link.addEventListener("click", clicked)
    const cell = link.closest<HTMLElement>("[role=gridcell]")
    expect(cell).not.toBeNull()
    cell?.focus()
    fireEvent.keyDown(cell as HTMLElement, { key: "Enter" })
    expect(clicked).toHaveBeenCalledTimes(1)
  })

  it("reports the hovered row and paints the highlighted one", async () => {
    const onHighlightRow = vi.fn()
    const { rerender, container } = render(
      <DataGrid
        label="Services"
        rows={rows}
        columns={columns}
        rowId={(row) => row.id}
        onHighlightRow={onHighlightRow}
      />
    )
    const cell = await screen.findByRole("gridcell", { name: "Search" })
    fireEvent.mouseOver(cell)
    await waitFor(() => expect(onHighlightRow).toHaveBeenLastCalledWith("b"))
    rerender(
      <DataGrid
        label="Services"
        rows={rows}
        columns={columns}
        rowId={(row) => row.id}
        highlightedRowId="b"
        onHighlightRow={onHighlightRow}
      />
    )
    await waitFor(() =>
      expect(
        container.querySelector<HTMLElement>('[role=row][row-id="b"]')?.style
          .backgroundColor
      ).toBe("var(--selection)")
    )
    fireEvent.pointerLeave(container.firstElementChild as HTMLElement)
    expect(onHighlightRow).toHaveBeenLastCalledWith(null)
  })

  it("shows the empty state instead of an empty grid", () => {
    render(
      <DataGrid
        label="Services"
        rows={[]}
        columns={columns}
        rowId={(row) => row.id}
        empty={<p>No services yet.</p>}
      />
    )
    expect(screen.getByText("No services yet.")).toBeInTheDocument()
    expect(screen.queryByRole("grid")).not.toBeInTheDocument()
  })

  describe("narrow", () => {
    it("draws each row as a card: the title, labelled values, then the action", async () => {
      vi.mocked(useNarrow).mockReturnValue(true)
      const { container } = render(
        <DataGrid
          label="Services"
          rows={rows}
          columns={columns}
          rowId={(row) => row.id}
        />
      )
      await waitFor(() =>
        expect(container.querySelectorAll("[data-narrow-row]")).toHaveLength(2)
      )
      expect(container.firstElementChild).toHaveAttribute(
        "data-layout",
        "narrow"
      )
      const card = container.querySelector<HTMLElement>(
        '[row-id="b"] [data-narrow-row]'
      )
      expect(card).not.toBeNull()
      const cells = [...(card?.children ?? [])].map((cell) => ({
        id: cell.getAttribute("data-cell"),
        label: cell.getAttribute("data-label"),
        text: cell.textContent,
      }))
      expect(cells).toEqual([
        { id: "name", label: null, text: "Search" },
        { id: "owner", label: "Owner", text: "Discovery" },
        { id: "note", label: "Note", text: "slow" },
        { id: "open", label: null, text: "Open Search" },
      ])
      expect(card?.firstElementChild).toHaveClass("basis-full", "font-medium")
      expect(card?.lastElementChild).toHaveClass("ml-auto")
    })

    it("places each card below the measured height of the one before", async () => {
      vi.mocked(useNarrow).mockReturnValue(true)
      const height = vi
        .spyOn(HTMLElement.prototype, "offsetHeight", "get")
        .mockImplementation(function (this: HTMLElement) {
          return this.hasAttribute("data-narrow-row") ? 120 : 0
        })
      const { container } = render(
        <DataGrid
          label="Services"
          rows={rows}
          columns={columns}
          rowId={(row) => row.id}
        />
      )
      await waitFor(() =>
        expect(
          container.querySelector<HTMLElement>('[row-id="b"]')?.style.transform
        ).toBe("translateY(120px)")
      )
      expect(
        container.querySelector<HTMLElement>('[row-id="a"]')?.style.height
      ).toBe("120px")
      height.mockRestore()
    })

    it("hides a value the row does not have", async () => {
      vi.mocked(useNarrow).mockReturnValue(true)
      const { container } = render(
        <DataGrid
          label="Services"
          rows={rows}
          columns={columns}
          rowId={(row) => row.id}
        />
      )
      await waitFor(() =>
        expect(
          container.querySelector('[row-id="a"] [data-cell="note"]')
        ).not.toBeNull()
      )
      const note = container.querySelector('[row-id="a"] [data-cell="note"]')
      expect(note).toBeEmptyDOMElement()
      expect(note).toHaveClass("empty:hidden")
    })

    it("hides the column headers and keeps one column so nothing scrolls sideways", async () => {
      vi.mocked(useNarrow).mockReturnValue(true)
      const { container } = render(
        <DataGrid
          label="Services"
          rows={rows}
          columns={columns}
          rowId={(row) => row.id}
        />
      )
      await waitFor(() =>
        expect(container.querySelectorAll("[data-narrow-row]")).toHaveLength(2)
      )
      expect(
        screen
          .getAllByRole("columnheader", { hidden: true })
          .map((header) => header.textContent?.trim())
      ).toEqual(["Services"])
      expect(
        container.querySelector<HTMLElement>(".ag-header-row")?.style.height
      ).toBe("0px")
    })
  })
})
