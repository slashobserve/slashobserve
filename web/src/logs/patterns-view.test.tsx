import { screen, waitFor, within } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { exploreKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { PatternsView } from "./patterns-view"
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
        "2 patterns cover 100% of 100 documents. Numbers and ids become <*>."
      )
    ).toBeTruthy()
    const grid = await screen.findByRole("grid")
    await waitFor(() => expect(grid).toHaveAttribute("aria-label", "Patterns"))
    const rows = within(grid).getAllByRole("row")
    expect(within(rows[1]).getByText("90")).toBeTruthy()
    expect(within(rows[1]).getByText("90%")).toBeTruthy()
    expect(within(rows[2]).getByText("10%")).toBeTruthy()
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
