import { fireEvent, screen, waitFor, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { useNarrow } from "@/components/data-grid/use-narrow"
import { renderWithProviders } from "@/test/render"

import { TraceTable } from "./trace-table"
import type { TraceRow } from "./trace-rows"

vi.mock("@/components/data-grid/use-narrow", () => ({
  useNarrow: vi.fn(() => false),
}))

afterEach(() => vi.mocked(useNarrow).mockReturnValue(false))

const rows: TraceRow[] = [
  {
    traceId: "a".repeat(32),
    start: 1_700_000_000_000,
    service: "checkout",
    span: "POST /checkout",
    durationMs: 3_020_004.4,
    spans: 1200,
    errors: 2,
  },
  {
    traceId: "b".repeat(32),
    start: 1_700_000_001_000,
    service: "cart",
    span: "GET /cart",
    durationMs: 12.5,
    spans: 3,
    errors: 0,
  },
]

function renderTable() {
  return renderWithProviders(
    <TraceTable
      rows={rows}
      datasourceUid="tempo"
      range={{ from: "now-1h", to: "now" }}
      selection={null}
      onSelectionChange={() => undefined}
    />,
    { route: { path: "/", url: "/" } }
  )
}

function serviceOrder() {
  return screen
    .getAllByRole("row")
    .filter((row) => row.hasAttribute("row-id"))
    .sort(
      (a, b) =>
        Number(a.getAttribute("row-index")) -
        Number(b.getAttribute("row-index"))
    )
    .map((row) => row.querySelector('[col-id="service"]')?.textContent)
}

describe("TraceTable", () => {
  it("gives every duration the width of the longest, so the text never runs under its bar", async () => {
    const { container } = renderTable()
    await waitFor(() =>
      expect(container.querySelectorAll('[data-cell="bar"]')).toHaveLength(2)
    )
    const texts = [...container.querySelectorAll('[data-cell="bar"]')].map(
      (cell) => cell.firstElementChild as HTMLElement
    )
    expect(texts.map((text) => text.textContent)).toEqual([
      "3,020,004.4 ms",
      "12.5 ms",
    ])
    for (const text of texts) {
      expect(text.style.width).toBe("14ch")
      expect(text).toHaveClass("shrink-0", "overflow-hidden", "tabular-nums")
      expect(text.nextElementSibling).toHaveClass("flex-1")
    }
  })

  it("sorts by duration first and resorts from a header", async () => {
    renderTable()
    const duration = await screen.findByRole("columnheader", {
      name: "Duration",
    })
    expect(duration).toHaveAttribute("aria-sort", "descending")
    expect(serviceOrder()).toEqual(["checkout", "cart"])

    const service = screen.getByRole("columnheader", { name: "Root service" })
    fireEvent.click(within(service).getByRole("button"))
    await waitFor(() =>
      expect(service).toHaveAttribute("aria-sort", "ascending")
    )
    expect(duration).toHaveAttribute("aria-sort", "none")
    await waitFor(() => expect(serviceOrder()).toEqual(["cart", "checkout"]))
    expect(screen.getByText(/Root service A to Z\./)).toBeInTheDocument()
  })

  it("hides a column from the Columns menu", async () => {
    const { user } = renderTable()
    await screen.findByRole("columnheader", { name: "Root span" })
    await user.click(screen.getByRole("button", { name: "Columns" }))
    await user.click(
      screen.getByRole("menuitemcheckbox", { name: "Root span" })
    )
    await waitFor(() =>
      expect(
        screen.queryByRole("columnheader", { name: "Root span" })
      ).not.toBeInTheDocument()
    )
  })

  it("opens each trace from its start time", async () => {
    renderTable()
    const link = await screen.findByRole("link", {
      name: "Open trace aaaaaaaa, POST /checkout, 3020.00 seconds",
    })
    expect(link.getAttribute("href")).toContain("a".repeat(32))
  })

  it("lists each trace as a card on a phone", async () => {
    vi.mocked(useNarrow).mockReturnValue(true)
    const { container } = renderTable()
    await waitFor(() =>
      expect(container.querySelectorAll("[data-narrow-row]")).toHaveLength(2)
    )
    const card = container.querySelector<HTMLElement>(
      `[row-id="${"a".repeat(32)}"] [data-narrow-row]`
    )
    expect(
      [...(card?.children ?? [])].map((cell) => cell.getAttribute("data-label"))
    ).toEqual([null, "Root span", "Duration", "Spans", "Errors", "Started"])
    expect(card?.querySelector('[data-cell="bar"]')).toHaveTextContent(
      "3,020,004.4 ms"
    )
    expect(
      within(card as HTMLElement).getByRole("link", {
        name: /Open trace aaaaaaaa/,
      })
    ).toBeInTheDocument()
  })
})
