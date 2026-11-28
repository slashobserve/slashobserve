import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import { createRef, useState } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { DataGrid, type DataGridColumn } from "./data-grid"
import { useNarrow } from "@/lib/use-narrow"

vi.mock("@/lib/use-narrow", () => ({ useNarrow: vi.fn(() => false) }))

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

  it("keeps the name of an action column for a screen reader only", async () => {
    render(
      <DataGrid
        label="Services"
        rows={rows}
        columns={columns}
        rowId={(row) => row.id}
      />
    )
    const open = await screen.findByRole("columnheader", { name: "Open" })
    expect(within(open).getByText("Open")).toHaveClass("sr-only")
    expect(
      within(screen.getByRole("columnheader", { name: "Note" })).getByText(
        "Note"
      )
    ).not.toHaveClass("sr-only")
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

  it("keeps what a cell holds when its column or its row changes", async () => {
    function Counter({ label }: { label: string }) {
      const [count, setCount] = useState(0)
      return (
        <button type="button" onClick={() => setCount(count + 1)}>
          {label}:{count}
        </button>
      )
    }
    const counted = (suffix: string): DataGridColumn<Row>[] => [
      {
        id: "name",
        header: "Name",
        cell: (row) => <Counter label={row.name} />,
      },
      { id: "suffix", header: "Suffix", cell: () => suffix },
    ]
    const props = { label: "Services", rowId: (row: Row) => row.id }
    const { rerender } = render(
      <DataGrid {...props} rows={rows.slice(0, 1)} columns={counted("a")} />
    )
    fireEvent.click(await screen.findByRole("button", { name: "Checkout:0" }))
    await screen.findByRole("button", { name: "Checkout:1" })

    rerender(
      <DataGrid {...props} rows={rows.slice(0, 1)} columns={counted("b")} />
    )
    await screen.findByText("b")
    expect(screen.getByRole("button", { name: "Checkout:1" })).toBeVisible()

    rerender(
      <DataGrid
        {...props}
        rows={[{ ...rows[0]!, name: "Basket" }]}
        columns={counted("b")}
      />
    )
    expect(
      await screen.findByRole("button", { name: "Basket:1" })
    ).toBeVisible()
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

  it("enters a cell that holds several controls on Enter, tabs between them, leaves on Tab from the cell, keeps arrows on the focused one, and Enter presses it", async () => {
    const pressed = vi.fn()
    const withActions: DataGridColumn<Row>[] = [
      columns[0] as DataGridColumn<Row>,
      {
        id: "actions",
        header: "Actions",
        action: true,
        cell: (row) => (
          <>
            {["Edit", "Copy", "Remove"].map((verb) => (
              <button
                key={verb}
                type="button"
                onClick={() => pressed(verb, row.id)}
              >
                {verb} {row.name}
              </button>
            ))}
          </>
        ),
      },
    ]
    render(
      <>
        <button type="button">Before</button>
        <DataGrid
          label="Services"
          rows={rows}
          columns={withActions}
          rowId={(row) => row.id}
        />
        <button type="button">After</button>
      </>
    )
    const edit = await screen.findByRole("button", { name: "Edit Checkout" })
    const copy = screen.getByRole("button", { name: "Copy Checkout" })
    const remove = screen.getByRole("button", { name: "Remove Checkout" })
    const cell = edit.closest<HTMLElement>("[role=gridcell]") as HTMLElement
    cell.focus()
    fireEvent.keyDown(cell, { key: "Enter" })
    expect(pressed).not.toHaveBeenCalled()
    expect(edit).toHaveFocus()
    fireEvent.keyDown(edit, { key: "Tab" })
    expect(copy).toHaveFocus()
    fireEvent.keyDown(copy, { key: "Tab" })
    expect(remove).toHaveFocus()
    fireEvent.keyDown(remove, { key: "ArrowDown" })
    expect(remove).toHaveFocus()
    fireEvent.keyDown(remove, { key: "Enter" })
    expect(pressed).toHaveBeenLastCalledWith("Remove", "a")
    expect(pressed).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(remove, { key: "Tab", shiftKey: true })
    expect(copy).toHaveFocus()
    fireEvent.keyDown(copy, { key: "Escape" })
    expect(cell).toHaveFocus()
    fireEvent.keyDown(cell, { key: "Tab" })
    expect(screen.getByRole("button", { name: "After" })).toHaveFocus()
    cell.focus()
    fireEvent.keyDown(cell, { key: "Tab", shiftKey: true })
    expect(screen.getByRole("button", { name: "Before" })).toHaveFocus()
    edit.focus()
    fireEvent.keyDown(edit, { key: "Tab", shiftKey: true })
    expect(screen.getByRole("button", { name: "Before" })).toHaveFocus()
    cell.focus()
    fireEvent.keyDown(cell, { key: "ArrowDown" })
    await waitFor(() =>
      expect(document.activeElement?.closest("[row-id]")).toHaveAttribute(
        "row-id",
        "b"
      )
    )
  })

  it("redraws a row whose content changes under the same id", async () => {
    const props = {
      label: "Services",
      columns,
      rowId: (row: Row) => row.id,
    }
    const { rerender } = render(<DataGrid {...props} rows={rows} />)
    await screen.findByRole("gridcell", { name: "Search" })
    rerender(
      <DataGrid
        {...props}
        rows={rows.map((row) =>
          row.id === "b" ? { ...row, name: "Search v2" } : row
        )}
      />
    )
    expect(
      await screen.findByRole("gridcell", { name: "Search v2" })
    ).toBeInTheDocument()
  })

  it("follows a new rowId on the same rows", async () => {
    const props = { label: "Services", rows, columns }
    const { container, rerender } = render(
      <DataGrid {...props} rowId={(row) => row.id} />
    )
    await screen.findByRole("gridcell", { name: "Checkout" })
    const ids = () =>
      [...container.querySelectorAll("[role=row][row-id]")].map((row) =>
        row.getAttribute("row-id")
      )
    expect(ids()).toEqual(["a", "b"])
    rerender(<DataGrid {...props} rowId={(row) => `service-${row.id}`} />)
    await waitFor(() => expect(ids()).toEqual(["service-a", "service-b"]))
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

  it("leaves the grid in one Tab from a cell instead of walking the other cells", async () => {
    render(
      <>
        <DataGrid
          label="Services"
          rows={rows}
          columns={columns.slice(0, 3)}
          rowId={(row) => row.id}
        />
        <button type="button">After</button>
      </>
    )
    const cell = (await screen.findByText("Checkout")).closest<HTMLElement>(
      "[role=gridcell]"
    ) as HTMLElement
    cell.focus()
    fireEvent.keyDown(cell, { key: "Tab" })
    expect(screen.getByRole("button", { name: "After" })).toHaveFocus()
  })

  it("hands Tab to the grid's own end guard when nothing follows it, so the next row's link is not the next stop", async () => {
    const { container } = render(
      <DataGrid
        label="Services"
        rows={rows}
        columns={columns}
        rowId={(row) => row.id}
      />
    )
    const cell = (await screen.findByText("Checkout")).closest<HTMLElement>(
      "[role=gridcell]"
    ) as HTMLElement
    cell.focus()
    const forward = new KeyboardEvent("keydown", {
      key: "Tab",
      bubbles: true,
      cancelable: true,
    })
    cell.dispatchEvent(forward)
    expect(forward.defaultPrevented).toBe(false)
    expect(document.activeElement).toHaveClass("ag-tab-guard-bottom")
    expect(container.contains(document.activeElement)).toBe(true)
  })

  it("marks the rows it is told are selected, and only those", async () => {
    const props = {
      label: "Services",
      rows,
      columns,
      rowId: (row: Row) => row.id,
    }
    const { container, rerender } = render(
      <DataGrid {...props} selectedRowIds={new Set(["b"])} />
    )
    const row = (id: string) =>
      container.querySelector<HTMLElement>(`[role=row][row-id="${id}"]`)
    await waitFor(() => expect(row("b")).toHaveClass("row-selected"))
    expect(row("a")).not.toHaveClass("row-selected")
    rerender(<DataGrid {...props} selectedRowIds={new Set(["a", "b"])} />)
    await waitFor(() => expect(row("a")).toHaveClass("row-selected"))
    rerender(<DataGrid {...props} selectedRowIds={new Set()} />)
    await waitFor(() => expect(row("b")).not.toHaveClass("row-selected"))
    expect(row("a")).not.toHaveClass("row-selected")
  })

  it("truncates a plain text cell with an ellipsis and the full text in a title, but not one that wraps", async () => {
    const long = "Observe Agent for adr-0042-long-service-name"
    const wrapping: DataGridColumn<Row>[] = [
      { id: "name", header: "Name", cell: (row) => row.name },
      {
        id: "owner",
        header: "Owner",
        autoHeight: true,
        cell: (row) => row.owner,
      },
    ]
    render(
      <DataGrid
        label="Services"
        rows={[{ id: "a", name: long, owner: long, note: "" }]}
        columns={wrapping}
        rowId={(row) => row.id}
      />
    )
    const cells = await screen.findAllByText(long)
    const [name, owner] = cells as [HTMLElement, HTMLElement]
    expect(name).toHaveClass("truncate")
    expect(name).not.toHaveAttribute("title")
    Object.defineProperty(name, "scrollWidth", {
      value: 200,
      configurable: true,
    })
    Object.defineProperty(name, "clientWidth", {
      value: 80,
      configurable: true,
    })
    fireEvent.pointerEnter(name)
    expect(name).toHaveAttribute("title", long)
    Object.defineProperty(name, "scrollWidth", {
      value: 80,
      configurable: true,
    })
    fireEvent.pointerEnter(name)
    expect(name).not.toHaveAttribute("title")
    expect(owner).not.toHaveClass("truncate")
    expect(owner).not.toHaveAttribute("title")
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

  it("names the grid by a heading and takes focus through its ref", async () => {
    const ref = createRef<HTMLDivElement>()
    render(
      <>
        <h2 id="services-heading">Services</h2>
        <DataGrid
          labelledBy="services-heading"
          ref={ref}
          busy
          rows={rows}
          columns={columns}
          rowId={(row) => row.id}
        />
      </>
    )
    const grid = await screen.findByRole("grid", { name: "Services" })
    expect(grid).not.toHaveAttribute("aria-label")
    expect(ref.current).toHaveAttribute("aria-busy", "true")
    ref.current?.focus()
    expect(ref.current).toHaveFocus()
  })

  it("draws a column's header content in place of its name, and redraws it when the content changes", async () => {
    const onChange = vi.fn()
    const withHeader = (label: string) => [
      {
        ...columns[0]!,
        headerCell: () => (
          <label>
            <input type="checkbox" onChange={onChange} />
            {label}
          </label>
        ),
      },
      ...columns.slice(1),
    ]
    const { rerender } = render(
      <DataGrid
        label="Services"
        rows={rows}
        columns={withHeader("Select all")}
        rowId={(row) => row.id}
      />
    )
    const header = await screen.findByRole("columnheader", {
      name: "Select all",
    })
    fireEvent.click(within(header).getByRole("checkbox"))
    expect(onChange).toHaveBeenCalledTimes(1)

    rerender(
      <DataGrid
        label="Services"
        rows={rows}
        columns={withHeader("Select none")}
        rowId={(row) => row.id}
      />
    )
    expect(
      await screen.findByRole("columnheader", { name: "Select none" })
    ).toBeInTheDocument()
  })

  it("draws the rows its predicate picks across the full width, in the wide layout too", async () => {
    const { container } = render(
      <DataGrid
        label="Services"
        rows={[...rows, { id: "g", name: "Group", owner: "", note: "" }]}
        columns={columns}
        rowId={(row) => row.id}
        fullWidthRow={{
          is: (row) => row.id === "g",
          render: (row) => <p>Section {row.name}</p>,
        }}
      />
    )
    await screen.findByText("Section Group")
    expect(container.querySelectorAll("[data-full-width-row]")).toHaveLength(1)
    expect(container.querySelectorAll("[data-narrow-row]")).toHaveLength(0)
    expect(
      screen.getByRole("gridcell", { name: "Checkout" })
    ).toBeInTheDocument()
  })

  describe("hideBelow", () => {
    const hiding = columns.map((column) =>
      column.id === "owner" ? { ...column, hideBelow: 900 } : column
    )

    function widthOfGrid(width: number) {
      return vi
        .spyOn(HTMLElement.prototype, "getBoundingClientRect")
        .mockImplementation(function (this: HTMLElement) {
          const grid = this.hasAttribute("data-layout")
          return { width: grid ? width : 0 } as DOMRect
        })
    }

    afterEach(() => vi.restoreAllMocks())

    it("drops the column while the grid is narrower than its limit", async () => {
      widthOfGrid(800)
      render(
        <DataGrid
          label="Services"
          rows={rows}
          columns={hiding}
          rowId={(row) => row.id}
        />
      )
      await screen.findByRole("columnheader", { name: "Name" })
      expect(screen.queryByRole("columnheader", { name: "Owner" })).toBeNull()
    })

    it("measures the grid once rows replace the empty state", async () => {
      widthOfGrid(800)
      const props = {
        label: "Services",
        columns: hiding,
        rowId: (row: (typeof rows)[number]) => row.id,
        empty: <p>No services</p>,
      }
      const { rerender } = render(<DataGrid {...props} rows={[]} />)
      expect(screen.getByText("No services")).toBeInTheDocument()
      rerender(<DataGrid {...props} rows={rows} />)
      await screen.findByRole("columnheader", { name: "Name" })
      expect(screen.queryByRole("columnheader", { name: "Owner" })).toBeNull()
    })

    it("keeps the column at its limit and while the width is unknown", async () => {
      widthOfGrid(900)
      const { unmount } = render(
        <DataGrid
          label="Services"
          rows={rows}
          columns={hiding}
          rowId={(row) => row.id}
        />
      )
      expect(
        await screen.findByRole("columnheader", { name: "Owner" })
      ).toBeInTheDocument()
      unmount()
      widthOfGrid(0)
      render(
        <DataGrid
          label="Services"
          rows={rows}
          columns={hiding}
          rowId={(row) => row.id}
        />
      )
      expect(
        await screen.findByRole("columnheader", { name: "Owner" })
      ).toBeInTheDocument()
    })
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

  it("drops the frame of its own when it sits inside a card", async () => {
    const { container } = render(
      <DataGrid
        label="Services"
        rows={rows}
        columns={columns}
        rowId={(row) => row.id}
        bare
      />
    )
    await screen.findByRole("gridcell", { name: "Checkout" })
    expect(container.firstElementChild).toHaveClass(
      "[&_.ag-root-wrapper]:border-0!",
      "[&_.ag-root-wrapper]:rounded-none!",
      "[&_.ag-styled-root]:[--ag-background-color:transparent]!",
      "[&_.ag-styled-root]:[--ag-header-background-color:transparent]!"
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

  describe("detail rows", () => {
    const detail = (open: string[]) => ({
      open: new Set(open),
      render: (row: Row) => <p>Detail of {row.name}</p>,
    })

    it("inserts a full-width row below each opened row and removes it when closed", async () => {
      const props = {
        label: "Services",
        rows,
        columns,
        rowId: (row: Row) => row.id,
      }
      const { container, rerender } = render(
        <DataGrid {...props} detail={detail([])} />
      )
      await screen.findByRole("gridcell", { name: "Checkout" })
      expect(screen.queryByText("Detail of Search")).not.toBeInTheDocument()
      rerender(<DataGrid {...props} detail={detail(["b"])} />)
      expect(await screen.findByText("Detail of Search")).toBeInTheDocument()
      const ids = [...container.querySelectorAll("[role=row][row-id]")].map(
        (row) => row.getAttribute("row-id")
      )
      expect(ids).toEqual(["a", "b", "b#detail"])
      expect(
        container.querySelector('[row-id="b#detail"] [data-detail-row]')
      ).not.toBeNull()
      rerender(<DataGrid {...props} detail={detail([])} />)
      await waitFor(() =>
        expect(screen.queryByText("Detail of Search")).not.toBeInTheDocument()
      )
    })

    it("tags the row and not its detail row with the row's attributes", async () => {
      const { container } = render(
        <DataGrid
          label="Services"
          rows={rows}
          columns={columns}
          rowId={(row) => row.id}
          rowAttributes={(row) => ({ "data-row-key": row.id })}
          detail={detail(["b"])}
        />
      )
      await screen.findByText("Detail of Search")
      expect(container.querySelectorAll("[data-row-key]")).toHaveLength(2)
      expect(
        container.querySelector('[row-id="b#detail"]')
      ).not.toHaveAttribute("data-row-key")
    })

    it("keeps the row highlighted over its detail row and clears it over the header", async () => {
      const onHighlightRow = vi.fn()
      const { container } = render(
        <DataGrid
          label="Services"
          rows={rows}
          columns={columns}
          rowId={(row) => row.id}
          detail={detail(["b"])}
          onHighlightRow={onHighlightRow}
        />
      )
      await screen.findByText("Detail of Search")
      fireEvent.pointerEnter(
        container.querySelector(
          '[row-id="b#detail"] [data-detail-row]'
        ) as Element
      )
      expect(onHighlightRow).toHaveBeenLastCalledWith("b")
      fireEvent.pointerOver(screen.getByRole("columnheader", { name: "Owner" }))
      expect(onHighlightRow).toHaveBeenLastCalledWith(null)
    })

    it("keeps the cards of the other rows when narrow", async () => {
      vi.mocked(useNarrow).mockReturnValue(true)
      const { container } = render(
        <DataGrid
          label="Services"
          rows={rows}
          columns={columns}
          rowId={(row) => row.id}
          detail={detail(["a"])}
        />
      )
      expect(await screen.findByText("Detail of Checkout")).toBeInTheDocument()
      await waitFor(() =>
        expect(container.querySelectorAll("[data-narrow-row]")).toHaveLength(2)
      )
    })
  })

  it("scrolls the revealed row into view when its id changes", async () => {
    const scrolled: (string | null)[] = []
    const original = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = function (this: Element) {
      scrolled.push(this.getAttribute("row-id"))
    }
    try {
      const props = {
        label: "Services",
        rows,
        columns,
        rowId: (row: Row) => row.id,
      }
      const { container, rerender } = render(<DataGrid {...props} />)
      await waitFor(() =>
        expect(container.querySelector('[row-id="b"]')).not.toBeNull()
      )
      rerender(<DataGrid {...props} revealRowId="b" />)
      await waitFor(() => expect(scrolled).toEqual(["b"]))
    } finally {
      Element.prototype.scrollIntoView = original
    }
  })

  describe("keepScrollOnPrepend", () => {
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
      keepScrollOnPrepend: true,
    }
    const added: Row[] = ["x", "y", "z"].map((id) => ({
      id,
      name: `New ${id}`,
      owner: "Payments",
      note: "",
    }))

    async function scrolled(container: HTMLElement, top: number) {
      const viewport = container.querySelector(
        ".ag-grid-viewport"
      ) as HTMLElement
      viewport.scrollTop = top
      fireEvent.scroll(viewport)
      await waitFor(() => expect(viewport.scrollTop).toBe(top))
      await new Promise((resolve) => setTimeout(resolve, 100))
      return viewport
    }

    it("keeps the same row in place when rows are prepended while scrolled away from the top", async () => {
      const { container, rerender } = render(
        <DataGrid {...props} rows={many} />
      )
      await screen.findByRole("gridcell", { name: "Service 0" })
      const viewport = await scrolled(container, 400)
      const rowTop = (id: string) =>
        Number(
          (
            container.querySelector(`[role=row][row-id="${id}"]`) as HTMLElement
          ).style.transform.match(/translateY\((\d+)px\)/)?.[1]
        )
      const before = viewport.scrollTop - rowTop("r10")
      rerender(<DataGrid {...props} rows={[...added, ...many]} />)
      await waitFor(() =>
        expect(container.querySelector('[row-id="x"]')).not.toBeNull()
      )
      await waitFor(() =>
        expect(viewport.scrollTop - rowTop("r10")).toBe(before)
      )
      expect(viewport.scrollTop).toBeGreaterThan(400)
    })

    it("keeps the anchor through a refresh that changes nothing, so a later prepend still holds the reading position", async () => {
      const { container, rerender } = render(
        <DataGrid {...props} rows={many} />
      )
      await screen.findByRole("gridcell", { name: "Service 0" })
      const viewport = await scrolled(container, 400)
      rerender(<DataGrid {...props} rows={many.map((row) => ({ ...row }))} />)
      await new Promise((resolve) => setTimeout(resolve, 100))
      expect(viewport.scrollTop).toBe(400)
      rerender(
        <DataGrid
          {...props}
          rows={[...added, ...many.map((row) => ({ ...row }))]}
        />
      )
      await waitFor(() =>
        expect(container.querySelector('[row-id="x"]')).not.toBeNull()
      )
      await waitFor(() => expect(viewport.scrollTop).toBeGreaterThan(400))
    })

    it("does not hold the viewport on an old row when the rows are replaced", async () => {
      const { container, rerender } = render(
        <DataGrid {...props} rows={many} />
      )
      await screen.findByRole("gridcell", { name: "Service 0" })
      const viewport = await scrolled(container, 400)
      const filtered = many.slice(60)
      rerender(<DataGrid {...props} rows={filtered} />)
      await waitFor(() =>
        expect(container.querySelector('[row-id="r60"]')).not.toBeNull()
      )
      await new Promise((resolve) => setTimeout(resolve, 100))
      expect(viewport.scrollTop).toBe(400)
    })

    it("lets new rows show at the top when the list is not scrolled", async () => {
      const { container, rerender } = render(
        <DataGrid {...props} rows={many} />
      )
      await screen.findByRole("gridcell", { name: "Service 0" })
      rerender(<DataGrid {...props} rows={[...added, ...many]} />)
      await waitFor(() =>
        expect(container.querySelector('[row-id="x"]')).not.toBeNull()
      )
      expect(
        (container.querySelector(".ag-grid-viewport") as HTMLElement).scrollTop
      ).toBe(0)
    })
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
      expect(card).toHaveClass("whitespace-normal")
      expect(card?.firstElementChild).toHaveClass("basis-full", "font-medium")
      for (const cell of card?.children ?? []) {
        if (cell.hasAttribute("data-label")) {
          expect(cell).toHaveClass(
            "wrap-anywhere",
            "before:shrink-0",
            "before:wrap-normal"
          )
        }
      }
      expect(card?.lastElementChild).toHaveClass("ml-auto")
    })

    it("presses the only control of a focused card on Enter", async () => {
      vi.mocked(useNarrow).mockReturnValue(true)
      const { container } = render(
        <DataGrid
          label="Services"
          rows={rows}
          columns={columns}
          rowId={(row) => row.id}
        />
      )
      const link = await screen.findByRole("link", { name: "Open Search" })
      const clicked = vi.fn((event: MouseEvent) => event.preventDefault())
      link.addEventListener("click", clicked)
      const card = container.querySelector<HTMLElement>('[row-id="b"]')
      expect(card).toHaveClass("ag-full-width-row")
      card?.focus()
      fireEvent.keyDown(card as HTMLElement, { key: "Enter" })
      expect(clicked).toHaveBeenCalledTimes(1)
    })

    it("redraws a card whose content changes under the same id", async () => {
      vi.mocked(useNarrow).mockReturnValue(true)
      const props = {
        label: "Services",
        columns,
        rowId: (row: Row) => row.id,
      }
      const { container, rerender } = render(
        <DataGrid {...props} rows={rows} />
      )
      await waitFor(() =>
        expect(
          container.querySelector('[row-id="b"] [data-cell="name"]')
        ).toHaveTextContent("Search")
      )
      rerender(
        <DataGrid
          {...props}
          rows={rows.map((row) =>
            row.id === "b" ? { ...row, name: "Search v2" } : row
          )}
        />
      )
      await waitFor(() =>
        expect(
          container.querySelector('[row-id="b"] [data-cell="name"]')
        ).toHaveTextContent("Search v2")
      )
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

    it("draws a picked row with its own content and the other rows as cards", async () => {
      vi.mocked(useNarrow).mockReturnValue(true)
      const { container } = render(
        <DataGrid
          label="Services"
          rows={[{ id: "g", name: "Group", owner: "", note: "" }, ...rows]}
          columns={columns}
          rowId={(row) => row.id}
          fullWidthRow={{
            is: (row) => row.id === "g",
            render: (row) => <p>Section {row.name}</p>,
          }}
        />
      )
      await screen.findByText("Section Group")
      await waitFor(() =>
        expect(container.querySelectorAll("[data-narrow-row]")).toHaveLength(2)
      )
      expect(container.querySelectorAll("[data-full-width-row]")).toHaveLength(
        1
      )
      expect(container.querySelector("[data-full-width-row]")).toHaveClass(
        "whitespace-normal"
      )
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

describe("DataGrid focus after removing a row", () => {
  const listed: Row[] = ["c", "a", "d", "b"].map((id) => ({
    id,
    name: id,
    owner: "",
    note: "",
  }))
  const withRemove: DataGridColumn<Row>[] = [
    { id: "name", header: "Name", cell: (row) => row.name },
    {
      id: "remove",
      header: "Remove",
      action: true,
      cell: (row) => (
        <button type="button" data-remove="">
          Remove {row.name}
        </button>
      ),
    },
  ]
  const props = {
    label: "Services",
    columns: withRemove,
    rowId: (row: Row) => row.id,
  }
  let listening = new AbortController()

  afterEach(() => {
    listening.abort()
    listening = new AbortController()
  })

  async function removeWhileFocused(removed: string) {
    const { rerender } = render(<DataGrid {...props} rows={listed} />)
    const button = await screen.findByRole("button", {
      name: `Remove ${removed}`,
    })
    button.focus()
    rerender(
      <DataGrid
        {...props}
        rows={listed}
        focusAfterRemoving={{ rowId: removed, control: "[data-remove]" }}
      />
    )
    const focused: string[] = []
    document.addEventListener(
      "focusin",
      (event) => {
        const target = event.target as HTMLElement
        focused.push(
          target.getAttribute("role") === "gridcell"
            ? `cell ${target.closest("[row-id]")?.getAttribute("row-id")}`
            : (target.textContent ?? "")
        )
      },
      { signal: listening.signal }
    )
    rerender(
      <DataGrid
        {...props}
        rows={listed.filter((row) => row.id !== removed)}
        focusAfterRemoving={{ rowId: removed, control: "[data-remove]" }}
      />
    )
    return focused
  }

  it.each([
    ["a middle row", "a", "d"],
    ["the first row", "c", "a"],
    ["the last row", "b", "d"],
  ])(
    "moves focus into the row that takes the place of %s in display order, after the grid restores its own focus",
    async (_, removed, neighbour) => {
      const focused = await removeWhileFocused(removed)

      await waitFor(() =>
        expect(
          screen.getByRole("button", { name: `Remove ${neighbour}` })
        ).toHaveFocus()
      )
      expect(focused).toEqual([`cell ${neighbour}`, `Remove ${neighbour}`])
    }
  )

  it("focuses the first cell of the row that takes the removed row's place when that row has no such control", async () => {
    const onlySome: DataGridColumn<Row>[] = [
      withRemove[0]!,
      {
        ...withRemove[1]!,
        cell: (row) =>
          row.id === "d" ? null : (
            <button type="button" data-remove="">
              Remove {row.name}
            </button>
          ),
      },
    ]
    const { rerender } = render(
      <DataGrid {...props} columns={onlySome} rows={listed} />
    )
    ;(await screen.findByRole("button", { name: "Remove a" })).focus()
    const removal = { rowId: "a", control: "[data-remove]" }
    rerender(
      <DataGrid
        {...props}
        columns={onlySome}
        rows={listed}
        focusAfterRemoving={removal}
      />
    )
    rerender(
      <DataGrid
        {...props}
        columns={onlySome}
        rows={listed.filter((row) => row.id !== "a")}
        focusAfterRemoving={removal}
      />
    )

    await waitFor(() =>
      expect(screen.getByRole("gridcell", { name: "d" })).toHaveFocus()
    )
  })

  it("leaves focus alone once it has moved out of the grid", async () => {
    const outside = document.createElement("button")
    document.body.append(outside)
    const { rerender } = render(<DataGrid {...props} rows={listed} />)
    await screen.findByRole("button", { name: "Remove a" })
    rerender(
      <DataGrid
        {...props}
        rows={listed}
        focusAfterRemoving={{ rowId: "a", control: "[data-remove]" }}
      />
    )
    outside.focus()
    rerender(
      <DataGrid
        {...props}
        rows={listed.filter((row) => row.id !== "a")}
        focusAfterRemoving={{ rowId: "a", control: "[data-remove]" }}
      />
    )

    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Remove a" })).toBeNull()
    )
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(outside).toHaveFocus()
    outside.remove()
  })
})
