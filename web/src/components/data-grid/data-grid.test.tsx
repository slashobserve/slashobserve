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
    const row = (id: string) =>
      container.querySelector<HTMLElement>(`[role=row][row-id="${id}"]`)
    await waitFor(() => expect(row("b")).toHaveClass("bg-selection!"))
    expect(row("a")).not.toHaveClass("bg-selection!")
    rerender(
      <DataGrid
        label="Services"
        rows={rows}
        columns={columns}
        rowId={(row) => row.id}
        highlightedRowId="a"
        onHighlightRow={onHighlightRow}
      />
    )
    await waitFor(() => expect(row("a")).toHaveClass("bg-selection!"))
    expect(row("b")).not.toHaveClass("bg-selection!")
    expect(cell).toBeInTheDocument()
    fireEvent.pointerLeave(container.firstElementChild as HTMLElement)
    expect(onHighlightRow).toHaveBeenLastCalledWith(null)
  })

  it("virtualises above 100 rows in a box that fills the page, and grows with its rows again below", async () => {
    const many = Array.from({ length: 150 }, (_, index) => ({
      id: `r${index}`,
      name: `Service ${index}`,
      owner: "Payments",
      note: "",
    }))
    const props = {
      label: "Services",
      columns,
      rowId: (row: Row) => row.id,
    }
    const { container, rerender } = render(<DataGrid {...props} rows={many} />)
    const box = container.firstElementChild as HTMLElement
    await screen.findByRole("gridcell", { name: "Service 0" })
    expect(box).toHaveAttribute("data-dom-layout", "normal")
    expect(box.style.height).toBe(`${window.innerHeight - 48}px`)
    expect(container.querySelector(".ag-root-wrapper")).toHaveClass(
      "ag-layout-normal"
    )
    expect(
      container.querySelectorAll("[role=row][row-id]").length
    ).toBeLessThan(many.length)

    rerender(<DataGrid {...props} rows={many.slice(0, 100)} />)
    await waitFor(() =>
      expect(container.querySelector(".ag-root-wrapper")).toHaveClass(
        "ag-layout-auto-height"
      )
    )
    expect(box).toHaveAttribute("data-dom-layout", "autoHeight")
    expect(box.style.height).toBe("")
  })

  it("sets the attributes a row asks for on the row itself, wide and narrow", async () => {
    const rowAttributes = (row: Row) => ({ "data-row-key": row.id })
    const { container, unmount } = render(
      <DataGrid
        label="Services"
        rows={rows}
        columns={columns}
        rowId={(row) => row.id}
        rowAttributes={rowAttributes}
      />
    )
    await screen.findByRole("gridcell", { name: "Checkout" })
    expect(
      container.querySelector('[role=row][data-row-key="b"]')
    ).toHaveAttribute("row-id", "b")
    unmount()

    vi.mocked(useNarrow).mockReturnValue(true)
    const narrow = render(
      <DataGrid
        label="Services"
        rows={rows}
        columns={columns}
        rowId={(row) => row.id}
        rowAttributes={rowAttributes}
      />
    )
    await waitFor(() =>
      expect(
        narrow.container.querySelector('[data-row-key="a"] [data-narrow-row]')
      ).not.toBeNull()
    )
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

    it("hides the column headers but keeps them sortable for a screen reader", async () => {
      vi.mocked(useNarrow).mockReturnValue(true)
      const onSort = vi.fn()
      const { container } = render(
        <DataGrid
          label="Services"
          rows={rows}
          columns={columns}
          rowId={(row) => row.id}
          sort={{ key: "name", direction: "ascending" }}
          onSort={onSort}
        />
      )
      await waitFor(() =>
        expect(container.querySelectorAll("[data-narrow-row]")).toHaveLength(2)
      )
      expect(
        container.querySelector<HTMLElement>(".ag-header-row")?.style.height
      ).toBe("0px")
      const name = screen.getByRole("columnheader", { name: "Name" })
      expect(name).toHaveAttribute("aria-sort", "ascending")
      fireEvent.keyDown(screen.getByRole("columnheader", { name: "Owner" }), {
        key: "Enter",
      })
      expect(onSort).toHaveBeenLastCalledWith("owner")
    })

    it("redraws the cards when a column is hidden", async () => {
      vi.mocked(useNarrow).mockReturnValue(true)
      const props = { label: "Services", rows, rowId: (row: Row) => row.id }
      const { container, rerender } = render(
        <DataGrid {...props} columns={columns} />
      )
      await waitFor(() =>
        expect(
          container.querySelector('[row-id="b"] [data-cell="owner"]')
        ).not.toBeNull()
      )
      rerender(
        <DataGrid
          {...props}
          columns={columns.filter((column) => column.id !== "owner")}
        />
      )
      await waitFor(() =>
        expect(
          container.querySelector('[row-id="b"] [data-cell="owner"]')
        ).toBeNull()
      )
      expect(
        container.querySelector('[row-id="b"] [data-cell="note"]')
      ).toHaveTextContent("slow")
    })

    it("reports the card under the pointer and paints the highlighted one", async () => {
      vi.mocked(useNarrow).mockReturnValue(true)
      const onHighlightRow = vi.fn()
      const props = {
        label: "Services",
        rows,
        columns,
        rowId: (row: Row) => row.id,
        onHighlightRow,
      }
      const { container, rerender } = render(<DataGrid {...props} />)
      await waitFor(() =>
        expect(container.querySelectorAll("[data-narrow-row]")).toHaveLength(2)
      )
      fireEvent.pointerEnter(
        container.querySelector('[row-id="b"] [data-narrow-row]') as Element
      )
      expect(onHighlightRow).toHaveBeenLastCalledWith("b")
      rerender(<DataGrid {...props} highlightedRowId="b" />)
      await waitFor(() =>
        expect(container.querySelector('[row-id="b"]')).toHaveClass(
          "bg-selection!"
        )
      )
    })
  })
})
