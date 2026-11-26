import { fireEvent, screen, waitFor, within } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { exploreKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { PatternsView, patternColumns } from "./patterns-view"
import type { LogPattern, LogPatterns } from "./patterns"
import { LogsWorkspaceContext, type LogsWorkspace } from "./workspace"

const range = { from: "now-1h", to: "now" }
const query = '{service_name="checkout"}'

function pattern(overrides: Partial<LogPattern>): LogPattern {
  return {
    pattern: "x",
    count: 1,
    sample: "",
    first: "",
    last: "",
    trend: [],
    level: "",
    ...overrides,
  }
}

function setup(patterns: LogPattern[], matched: number) {
  const result: LogPatterns = {
    source: "server",
    lines_sampled: matched,
    lines_matched: matched,
    patterns,
    truncated: false,
  }
  const update = vi.fn()
  const setDraft = vi.fn()
  const workspace = {
    datasource: {
      uid: "ds",
      type: "loki",
      name: "ds",
      healthy: true,
      checked_at: "2026-11-07T12:00:00Z",
      dashboard_count: 0,
    },
    search: { q: query, view: "patterns" },
    effectiveQuery: query,
    range,
    update,
    setDraft,
  } as unknown as LogsWorkspace
  const queryClient = testQueryClient()
  queryClient.setQueryData(
    exploreKeys.patterns("ds", { query, ...range, limit: 20 }),
    result
  )
  const view = renderWithProviders(
    <LogsWorkspaceContext value={workspace}>
      <PatternsView />
    </LogsWorkspaceContext>,
    { queryClient }
  )
  return { ...view, update, setDraft }
}

describe("PatternsView", () => {
  it("lists each pattern with its count and share under a heading", async () => {
    setup(
      [
        pattern({ pattern: "paid order <*>", count: 90, level: "info" }),
        pattern({ pattern: "card declined <*>", count: 10, level: "error" }),
      ],
      100
    )
    expect(
      screen.getByText(
        "2 patterns cover 100% of the newest 100 lines. Numbers and ids become <*>."
      )
    ).toBeTruthy()
    const grid = await screen.findByRole("grid")
    await waitFor(() => expect(grid).toHaveAttribute("aria-label", "Patterns"))
    const rows = within(grid).getAllByRole("row")
    expect(within(rows[1]).getByText("90")).toBeTruthy()
    expect(within(rows[1]).getByText("90%")).toBeTruthy()
    expect(within(rows[2]).getByText("10%")).toBeTruthy()
  })

  it("says a server heading counts the newest lines, not every line", () => {
    setup([pattern({ pattern: "paid order <*>", count: 258 })], 258)
    expect(
      screen.getByText(
        "1 pattern covers 100% of the newest 258 lines. Numbers and ids become <*>."
      )
    ).toBeTruthy()
  })

  it("filters to a pattern through the query and returns to the lines", async () => {
    const { user, update, setDraft } = setup(
      [pattern({ pattern: "paid order <*>", count: 5 })],
      5
    )
    await user.click(
      await screen.findByRole("button", { name: "Filter to this pattern" })
    )
    const next = `${query} |~ "paid order .*?"`
    expect(setDraft).toHaveBeenCalledWith(next)
    expect(update).toHaveBeenCalledWith({ q: next, view: undefined })
  })

  it("filters a pattern out", async () => {
    const { user, update } = setup(
      [pattern({ pattern: "paid order <*>", count: 5 })],
      5
    )
    await user.click(
      await screen.findByRole("button", { name: "Filter out this pattern" })
    )
    expect(update).toHaveBeenCalledWith({
      q: `${query} !~ "paid order .*?"`,
      view: undefined,
    })
  })

  it("shows a sample line only after Samples is pressed", async () => {
    const { user } = setup(
      [
        pattern({
          pattern: "paid order <*>",
          count: 5,
          sample: "paid order 7",
        }),
      ],
      5
    )
    const show = await screen.findByRole("button", {
      name: "Show a sample line of this pattern",
    })
    expect(screen.queryByText("paid order 7")).toBeNull()
    await user.click(show)
    expect(await screen.findByText("paid order 7")).toBeTruthy()
    await user.click(
      screen.getByRole("button", { name: "Hide a sample line of this pattern" })
    )
    await waitFor(() => expect(screen.queryByText("paid order 7")).toBeNull())
  })

  it("reaches Filter, Filter out and Samples from the Actions cell by Enter then Tab", async () => {
    setup(
      [
        pattern({
          pattern: "paid order <*>",
          count: 5,
          sample: "paid order 7",
        }),
      ],
      5
    )
    const filter = await screen.findByRole("button", {
      name: "Filter to this pattern",
    })
    const cell = filter.closest<HTMLElement>("[role=gridcell]")
    expect(cell).not.toBeNull()
    cell?.focus()
    const stops = [
      filter,
      screen.getByRole("button", { name: "Filter out this pattern" }),
      screen.getByRole("button", {
        name: "Show a sample line of this pattern",
      }),
    ]
    fireEvent.keyDown(cell as HTMLElement, { key: "Enter" })
    expect(document.activeElement).toBe(stops[0])
    for (const stop of stops.slice(1)) {
      fireEvent.keyDown(document.activeElement as Element, { key: "Tab" })
      expect(document.activeElement).toBe(stop)
    }
  })

  it("offers Filter and Filter out on every row and Samples only where the row has a sample line", async () => {
    setup(
      [
        pattern({ pattern: "with sample", count: 9, sample: "with sample 1" }),
        pattern({ pattern: "without sample", count: 1 }),
      ],
      10
    )
    await screen.findByText("with sample")
    const actions = (name: string) => {
      const row = screen.getByText(name).closest<HTMLElement>("[role=row]")
      expect(row).not.toBeNull()
      return within(row as HTMLElement)
        .getAllByRole("button")
        .map((button) => button.textContent)
    }
    expect(actions("with sample")).toEqual(["Filter", "Filter out", "Samples"])
    expect(actions("without sample")).toEqual(["Filter", "Filter out"])
  })

  it("folds the patterns under 1% until Show all", async () => {
    const major = Array.from({ length: 9 }, (_, index) =>
      pattern({ pattern: `major ${index}`, count: 1000 })
    )
    const minor = [pattern({ pattern: "rare one", count: 1 })]
    const { user } = setup([...major, ...minor], 9001)
    await screen.findByText("major 0")
    expect(screen.queryByText("rare one")).toBeNull()
    expect(
      screen.getByText("9 of 10 patterns shown, 1 cover under 1% each")
    ).toBeTruthy()
    await user.click(screen.getByRole("button", { name: "Show all" }))
    expect(await screen.findByText("rare one")).toBeTruthy()
  })
})

describe("patternColumns", () => {
  it("fits the 868 px content area at a 1440 px window with the pattern column flexing", () => {
    const columns = patternColumns({
      total: 1,
      largest: 1,
      expanded: new Set(),
      onToggle: () => {},
      rewrite: () => null,
      onApply: () => {},
    })
    const [pattern, ...rest] = columns
    expect(pattern.width).toBeUndefined()
    expect(pattern.flex).toBeGreaterThan(0)
    expect(rest.every((column) => column.width !== undefined)).toBe(true)
    const fixed = rest.reduce((sum, column) => sum + (column.width ?? 0), 0)
    expect(fixed + (pattern.minWidth ?? 0)).toBeLessThanOrEqual(868)
  })
})
