import { act, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import type { DataPanel } from "@/spec"

import { panelOf, renderPublishing, stubRects } from "../anchor-harness"
import type { PreparedFrame } from "../transforms"
import type { TableAnchorLayout } from "./anchors"
import TablePanel from "./table-panel"

type Field = PreparedFrame["fields"][number]

const text = (name: string, values: (string | null)[]): Field => ({
  name,
  type: "string",
  values,
})

const number = (name: string, values: (number | null)[]): Field => ({
  name,
  type: "number",
  values,
})

function frameOf(...fields: Field[]): PreparedFrame[] {
  return [{ ref: "A", fields }]
}

const routes = frameOf(
  text("route", ["/cart", "/pay", "/home"]),
  number("requests", [5, 50, 20])
)

function many(count: number): PreparedFrame[] {
  return frameOf(
    text(
      "route",
      Array.from({ length: count }, (_, index) => `/r${index}`)
    ),
    number(
      "requests",
      Array.from({ length: count }, (_, index) => index)
    )
  )
}

function tableOf(
  display: Record<string, unknown> = {},
  config: Partial<DataPanel> = {}
): DataPanel {
  return { ...panelOf("table", display), ...config } as DataPanel
}

function show(
  panel: DataPanel,
  frames: PreparedFrame[] = routes,
  size: { width?: number; height?: number } = {}
) {
  return renderPublishing(TablePanel, panel, frames, size)
}

function cellsOf(column: string): string[] {
  return Array.from(
    document.querySelectorAll<HTMLElement>(`[data-column="${column}"]`)
  ).map((cell) => cell.textContent ?? "")
}

function rowOf(key: string): HTMLElement {
  const row = document.querySelector<HTMLElement>(`[role=row][row-id="${key}"]`)
  if (row === null) throw new Error(`no row ${key}`)
  return row
}

function gridCellOf(element: Element): HTMLElement {
  const cell = element.closest<HTMLElement>("[role=gridcell]")
  if (cell === null) throw new Error("not in a grid cell")
  return cell
}

const gridErrors = vi.spyOn(console, "error")
const gridWarnings = vi.spyOn(console, "warn")

afterEach(() => {
  const reports = [...gridErrors.mock.calls, ...gridWarnings.mock.calls]
    .map((call) => String(call[0]))
    .filter((message) => message.includes("AG Grid"))
  gridErrors.mockClear()
  gridWarnings.mockClear()
  vi.restoreAllMocks()
  expect(reports).toEqual([])
})

describe("table panel", () => {
  it("draws a labelled region with a header and a cell per value", async () => {
    show(tableOf())
    expect(
      screen.getByRole("region", { name: "Panel table" })
    ).toBeInTheDocument()
    await waitFor(() =>
      expect(screen.getByRole("grid")).toHaveAttribute("aria-label", "Panel")
    )
    await waitFor(() => expect(cellsOf("route")).toHaveLength(3))
    expect(cellsOf("route")).toEqual(["/cart", "/pay", "/home"])
    expect(
      screen
        .getAllByRole("columnheader")
        .map((header) => header.textContent?.trim())
    ).toEqual(["route", "requests"])
  })

  it("sorts text in natural order", async () => {
    show(
      tableOf({ sortBy: "host" }),
      frameOf(text("host", ["web-10", "web-2", "web-1"]))
    )
    await waitFor(() =>
      expect(cellsOf("host")).toEqual(["web-1", "web-2", "web-10"])
    )
  })

  it("sorts by sortBy and sortDesc, and cycles the sort from the header", async () => {
    show(tableOf({ sortBy: "requests", sortDesc: true }))
    await waitFor(() => expect(cellsOf("route")).toHaveLength(3))
    const requests = screen.getByRole("columnheader", { name: /requests/ })
    expect(requests).toHaveAttribute("aria-sort", "descending")
    expect(screen.getByRole("columnheader", { name: /route/ })).toHaveAttribute(
      "aria-sort",
      "none"
    )
    await waitFor(() =>
      expect(cellsOf("route")).toEqual(["/pay", "/home", "/cart"])
    )
    const user = userEvent.setup()
    await user.click(within(requests).getByRole("button", { name: /requests/ }))
    await waitFor(() => expect(requests).toHaveAttribute("aria-sort", "none"))
    await user.click(within(requests).getByRole("button", { name: /requests/ }))
    await waitFor(() =>
      expect(requests).toHaveAttribute("aria-sort", "ascending")
    )
    await waitFor(() =>
      expect(cellsOf("route")).toEqual(["/cart", "/home", "/pay"])
    )
  })

  it("names each column's type when typeIcons is on", async () => {
    show(tableOf({ typeIcons: true }))
    await waitFor(() => expect(cellsOf("route")).toHaveLength(3))
    expect(screen.getAllByRole("button").map((b) => b.textContent)).toEqual([
      "route, text",
      "requests, number",
    ])
  })

  it("pins the frozen columns to the left", async () => {
    show(tableOf({ frozenColumns: 1 }))
    await waitFor(() => expect(cellsOf("route")).toHaveLength(3))
    const route = screen.getByRole("columnheader", { name: /route/ })
    const requests = screen.getByRole("columnheader", { name: /requests/ })
    expect(route.closest(".ag-grid-pinned-left-cells")).not.toBeNull()
    expect(requests.closest(".ag-grid-pinned-left-cells")).toBeNull()
  })

  it("adds a footer row per reducer and the Rows count", async () => {
    show(tableOf({ footer: ["sum"], footerCountRows: true }))
    await waitFor(() =>
      expect(
        Array.from(document.querySelectorAll("[data-footer-label]")).map(
          (label) => label.getAttribute("data-footer-label")
        )
      ).toEqual(["Total", "Rows"])
    )
    const total = rowOf("footer:0")
    expect(total.closest(".ag-grid-pinned-bottom-rows")).not.toBeNull()
    expect(total.textContent).toContain("75")
    expect(rowOf("footer:1").textContent).toContain("3")
  })

  it("pages the rows with Previous and Next", async () => {
    show(tableOf({ pagination: true }), many(30), { height: 200 })
    const status = await screen.findByRole("status")
    await waitFor(() => expect(status).toHaveTextContent(/^Page 1 of \d+$/))
    const pages = Number(/of (\d+)/.exec(status.textContent ?? "")?.[1])
    expect(pages).toBeGreaterThan(1)
    expect(screen.getByRole("button", { name: "Previous page" })).toBeDisabled()
    await waitFor(() => expect(cellsOf("route")[0]).toBe("/r0"))
    const user = userEvent.setup()
    await user.click(screen.getByRole("button", { name: "Next page" }))
    await waitFor(() => expect(status).toHaveTextContent(`Page 2 of ${pages}`))
    expect(cellsOf("route")).not.toContain("/r0")
    expect(screen.getByRole("button", { name: "Previous page" })).toBeEnabled()
  })

  it("shows the first 1000 rows without pagination and says so", async () => {
    show(tableOf(), many(1001))
    expect(
      await screen.findByText("Showing 1,000 of 1,001 rows")
    ).toBeInTheDocument()
    expect(screen.queryByRole("status")).not.toBeInTheDocument()
  })

  it.each([
    ["sm", "25px"],
    ["md", "33px"],
    ["lg", "41px"],
  ])("gives %s rows a fixed height of %s", async (cellHeight, height) => {
    show(tableOf({ cellHeight }))
    await waitFor(() => expect(cellsOf("route")).toHaveLength(3))
    expect(rowOf("0:0").style.height).toBe(height)
  })

  it("lets auto rows grow with padding and caps them at maxRowHeight", async () => {
    show(tableOf({ cellHeight: "auto", maxRowHeight: 60 }))
    await waitFor(() => expect(cellsOf("route")).toHaveLength(3))
    const cell = document.querySelector<HTMLElement>('[data-column="route"]')
    expect(cell).toHaveClass("py-1")
    expect(cell).not.toHaveClass("h-full")
    const capped = cell?.querySelector<HTMLElement>("[style]")
    expect(capped?.style.maxHeight).toBe("60px")
  })

  it("lets rows grow when only a later frame's field wraps", async () => {
    const merged: PreparedFrame[] = [
      { ref: "A", fields: [text("route", ["/cart"])] },
      { ref: "B", fields: [text("route", ["/a long route that wraps"])] },
    ]
    show(
      tableOf({}, {
        overrides: [{ match: { query: "B" }, cell: { wrap: true } }],
      } as Partial<DataPanel>),
      merged
    )
    await waitFor(() => expect(cellsOf("route")).toHaveLength(2))
    const [first, second] = Array.from(
      document.querySelectorAll<HTMLElement>('[data-column="route"]')
    )
    expect(first).not.toHaveClass("h-full")
    expect(second).not.toHaveClass("h-full")
    expect(second.firstElementChild).toHaveClass("whitespace-normal")
    expect(first.firstElementChild).toHaveClass("whitespace-nowrap")
  })

  it("keeps fixed rows at full height without a cap", async () => {
    show(tableOf({ maxRowHeight: 60 }))
    await waitFor(() => expect(cellsOf("route")).toHaveLength(3))
    const cell = document.querySelector<HTMLElement>('[data-column="route"]')
    expect(cell).toHaveClass("h-full")
    expect(cell?.querySelector("[style]")).toBeNull()
  })

  it("filters rows from a column's filter menu", async () => {
    show(tableOf({}, { filterable: true } as Partial<DataPanel>))
    await waitFor(() => expect(cellsOf("route")).toHaveLength(3))
    const user = userEvent.setup()
    await user.click(screen.getByRole("button", { name: "Filter route" }))
    const values = await screen.findByRole("list", { name: "Values of route" })
    await user.click(within(values).getByRole("checkbox", { name: "/pay" }))
    await waitFor(() => expect(cellsOf("route")).toEqual(["/pay"]))
    expect(
      screen.getByRole("button", { name: "Filter route (1 selected)" })
    ).toBeInTheDocument()
  })

  it("draws each cell type through its own cell", async () => {
    show(
      tableOf({}, {
        overrides: [
          { match: { field: "route" }, cell: { type: "pill" } },
          { match: { field: "requests" }, cell: { type: "gauge" } },
        ],
      } as Partial<DataPanel>)
    )
    await waitFor(() => expect(cellsOf("route")).toHaveLength(3))
    const types = (column: string) =>
      Array.from(
        document.querySelectorAll<HTMLElement>(`[data-column="${column}"]`)
      ).map((cell) => cell.dataset.cell)
    expect(types("route")).toEqual(["pill", "pill", "pill"])
    expect(types("requests")).toEqual(["gauge", "gauge", "gauge"])
  })

  it("fills a background cell and marks its threshold", async () => {
    show(
      tableOf({}, {
        thresholds: [{ color: "ok" }, { value: 10, color: "warning" }],
        overrides: [
          { match: { field: "requests" }, cell: { type: "background" } },
        ],
      } as Partial<DataPanel>)
    )
    await waitFor(() => expect(cellsOf("requests")).toHaveLength(3))
    const [low, high] = Array.from(
      document.querySelectorAll<HTMLElement>('[data-column="requests"]')
    )
    expect(gridCellOf(low).style.backgroundColor).toBe("var(--threshold-ok)")
    expect(gridCellOf(high).style.backgroundColor).toBe(
      "var(--threshold-warning)"
    )
    expect(high).toHaveAttribute("data-threshold", "warning")
    const route = document.querySelector('[data-column="route"]') as Element
    expect(gridCellOf(route).style.backgroundColor).toBe("")
  })

  it("fills the whole row from a whole-row background cell", async () => {
    show(
      tableOf({}, {
        thresholds: [{ color: "ok" }, { value: 10, color: "warning" }],
        overrides: [
          {
            match: { field: "requests" },
            cell: { type: "background", wholeRow: true },
          },
        ],
      } as Partial<DataPanel>)
    )
    await waitFor(() => expect(cellsOf("route")).toHaveLength(3))
    const route = document.querySelectorAll('[data-column="route"]')
    expect(gridCellOf(route[0]).style.backgroundColor).toBe(
      "var(--threshold-ok)"
    )
    expect(gridCellOf(route[1]).style.backgroundColor).toBe(
      "var(--threshold-warning)"
    )
  })

  it("opens a nested frame below its row", async () => {
    const inner = (values: number[]): PreparedFrame => ({
      ref: "A",
      name: "rows",
      fields: [number("lat", values)],
    })
    const grouped = frameOf(text("host", ["a", "b"]), {
      name: "Nested frames",
      type: "other",
      values: [inner([1, 2]), null],
      nested: { headers: true },
    })
    show(tableOf(), grouped)
    const expand = await screen.findByRole("button", { name: "Expand a" })
    expect(expand).toHaveAttribute("aria-expanded", "false")
    expect(screen.queryByRole("button", { name: "Expand b" })).toBeNull()
    const user = userEvent.setup()
    await user.click(expand)
    const nested = await screen.findByRole("grid", { name: "a rows" })
    await waitFor(() =>
      expect(within(nested).getAllByRole("gridcell")).toHaveLength(2)
    )
    expect(
      within(nested).getByRole("columnheader", { name: "lat" })
    ).toBeInTheDocument()
    const collapse = screen.getByRole("button", { name: "Collapse a" })
    expect(collapse).toHaveAttribute("aria-expanded", "true")
    expect(
      document.getElementById(collapse.getAttribute("aria-controls") ?? "")
    ).toContainElement(nested)
    await user.click(collapse)
    await waitFor(() =>
      expect(screen.queryByRole("grid", { name: "a rows" })).toBeNull()
    )
  })

  it("keeps an opened row's detail on its page and the page count", async () => {
    const inner = (): PreparedFrame => ({
      ref: "A",
      name: "rows",
      fields: [number("lat", [1, 2])],
    })
    const hosts = Array.from({ length: 10 }, (_, index) => `h${index}`)
    const grouped = frameOf(text("host", hosts), {
      name: "Nested frames",
      type: "other",
      values: hosts.map(inner),
      nested: { headers: false },
    })
    show(tableOf({ pagination: true }), grouped, { height: 200 })
    const status = await screen.findByRole("status")
    await waitFor(() => expect(status).toHaveTextContent(/^Page 1 of \d+$/))
    const before = status.textContent
    expect(before).toBe("Page 1 of 2")
    await waitFor(() => expect(cellsOf("host").length).toBeGreaterThan(1))
    const shown = cellsOf("host")
    const last = shown[shown.length - 1]
    const user = userEvent.setup()
    await user.click(screen.getByRole("button", { name: `Expand ${last}` }))
    expect(
      await screen.findByRole("grid", { name: `${last} rows` })
    ).toBeInTheDocument()
    expect(status.textContent).toBe(before)
    expect(cellsOf("host")).toEqual(shown)
  })

  it("keeps the header for screen readers when it is hidden", async () => {
    show(tableOf({ header: false, sortBy: "requests" }))
    await waitFor(() => expect(cellsOf("route")).toHaveLength(3))
    const route = screen.getByRole("columnheader", { name: "route" })
    expect(route).not.toHaveAttribute("aria-sort")
    expect(within(route).queryByRole("button")).toBeNull()
    const headerRow = route.closest<HTMLElement>("[role=row]")
    expect(headerRow?.style.height).toBe("0px")
  })
})

describe("table anchors", () => {
  function stubGrid() {
    return stubRects((element) => {
      if (element.getAttribute("role") === "region") {
        return { x: 0, y: 0, width: 400, height: 240 }
      }
      const column = element.getAttribute("col-id")
      if (element.getAttribute("role") === "columnheader" && column) {
        const left = column.endsWith("route") ? 0 : 200
        return { x: left, y: 0, width: 200, height: 32 }
      }
      const key = element.getAttribute("row-id")
      if (element.getAttribute("role") === "row" && key) {
        if (key.startsWith("footer:")) {
          return { x: 0, y: 215, width: 400, height: 25 }
        }
        const index = Number(key.replace("0:", ""))
        return { x: 0, y: 32 + index * 25, width: 400, height: 25 }
      }
      return undefined
    })
  }

  it("publishes the drawn columns, header and rows", async () => {
    stubGrid()
    const { store } = show(tableOf())
    await waitFor(() => expect(cellsOf("route")).toHaveLength(3))
    await waitFor(() => {
      const layout = store.current()?.layout as TableAnchorLayout
      expect(layout.rows).toHaveLength(3)
    })
    const published = store.current()
    expect(published?.type).toBe("table")
    const layout = published?.layout as TableAnchorLayout
    expect(layout.body).toEqual({ width: 400, height: 240 })
    expect(layout.gridBottom).toBe(240)
    expect(layout.header).toEqual({ y: 0, height: 32 })
    expect(layout.columns).toEqual([
      { name: "route", left: 0, width: 200, dimension: true },
      { name: "requests", left: 200, width: 200, dimension: false },
    ])
    expect(layout.rows.map(({ top, cells }) => ({ top, cells }))).toEqual([
      { top: 32, cells: ["/cart", "5"] },
      { top: 57, cells: ["/pay", "50"] },
      { top: 82, cells: ["/home", "20"] },
    ])
    expect(layout.elsewhere).toEqual([])
  })

  it("leaves rows under the footer to elsewhere", async () => {
    stubGrid()
    const { store } = show(tableOf({ footer: ["sum"] }), many(9))
    await waitFor(() => expect(cellsOf("route")).toHaveLength(9))
    await waitFor(() => {
      const layout = store.current()?.layout as TableAnchorLayout
      expect(layout.rows).toHaveLength(7)
    })
    const layout = store.current()?.layout as TableAnchorLayout
    expect(layout.elsewhere?.map(({ key }) => key)).toEqual(["0:7", "0:8"])
  })

  it("reveals a row on another page", async () => {
    const { store } = show(tableOf({ pagination: true }), many(30), {
      height: 200,
    })
    const status = await screen.findByRole("status")
    await waitFor(() => expect(status).toHaveTextContent(/^Page 1 of/))
    await waitFor(() =>
      expect((store.current()?.layout as TableAnchorLayout).reveal).toBeTypeOf(
        "function"
      )
    )
    act(() => {
      ;(store.current()?.layout as TableAnchorLayout).reveal?.("0:29")
    })
    await waitFor(() => expect(cellsOf("route")).toContain("/r29"))
    expect(status).not.toHaveTextContent(/^Page 1 of/)
  })
})
