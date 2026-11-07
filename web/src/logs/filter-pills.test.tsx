import { QueryClientProvider } from "@tanstack/react-query"
import { screen, within } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { datasourceKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { FilterPills } from "./filter-pills"
import { applyPills, decodePills, encodePills, type Pill } from "./pills"
import type { LogsSearch } from "./search"
import { LogsWorkspaceContext, type LogsWorkspace } from "./workspace"

const range = { from: "now-1h", to: "now" }

const seeded: Pill[] = [
  {
    id: "0",
    kind: "is",
    field: "service.name",
    value: "checkout",
    negate: false,
    pinned: false,
    paused: false,
  },
  {
    id: "1",
    kind: "is",
    field: "host.name",
    value: "canary-*",
    negate: true,
    pinned: true,
    paused: false,
  },
  {
    id: "2",
    kind: "is",
    field: "kubernetes.namespace",
    value: "staging",
    negate: false,
    pinned: false,
    paused: false,
  },
]

function workspaceOf(
  type: string,
  search: LogsSearch,
  mocks: {
    update: LogsWorkspace["update"]
    run: LogsWorkspace["run"]
    announce: LogsWorkspace["announce"]
  }
) {
  return {
    datasource: {
      uid: "ds",
      type,
      name: "ds",
      healthy: true,
      checked_at: "2026-11-07T12:00:00Z",
      dashboard_count: 0,
    },
    search,
    range,
    ...mocks,
  } as unknown as LogsWorkspace
}

function setup(
  options: { type?: string; q?: string | undefined; pills?: Pill[] } = {}
) {
  const mocks = { update: vi.fn(), run: vi.fn(), announce: vi.fn() }
  const type = options.type ?? "elasticsearch"
  const searchOf = (q: string | undefined): LogsSearch => ({
    q,
    f: encodePills(options.pills ?? seeded),
  })
  const queryClient = testQueryClient()
  queryClient.setQueryData(datasourceKeys.labels("ds", range), {
    labels: ["service.name", "host.name", "log.level"],
    truncated: false,
  })
  queryClient.setQueryData(
    datasourceKeys.labelValues("ds", "log.level", range),
    { values: ["error", "warn", "info"], truncated: false }
  )
  const view = (q: string | undefined) => (
    <LogsWorkspaceContext value={workspaceOf(type, searchOf(q), mocks)}>
      <FilterPills />
    </LogsWorkspaceContext>
  )
  const rendered = renderWithProviders(
    view("q" in options ? options.q : "timeout"),
    { queryClient }
  )
  return {
    ...rendered,
    ...mocks,
    changeQuery: (q: string) =>
      rendered.rerender(
        <QueryClientProvider client={queryClient}>
          {view(q)}
        </QueryClientProvider>
      ),
  }
}

function lastFilters(update: ReturnType<typeof vi.fn>): Pill[] {
  const call = update.mock.calls.at(-1)
  return decodePills((call?.[0] as { f?: string }).f)
}

describe("FilterPills", () => {
  it("shows a pill for each filter with the caption", () => {
    setup()
    const list = screen.getByRole("list", { name: "Filters" })
    const labels = within(list)
      .getAllByRole("button")
      .map((button) => button.textContent)
    expect(labels).toEqual([
      "service.name: checkout",
      "NOT host.name: canary-*",
      "kubernetes.namespace: staging",
    ])
    expect(
      screen.getByText(
        "Filters are added to the query in KQL. Pinned ones stay when the query changes."
      )
    ).toBeVisible()
    expect(screen.getByRole("button", { name: "Add filter" })).toBeVisible()
  })

  it("names the query language of the datasource", () => {
    setup({ type: "loki" })
    expect(screen.getByText(/added to the query in LogQL/)).toBeVisible()
  })

  it("marks negated, pinned and paused pills for styling", () => {
    setup({
      pills: [
        { ...seeded[1] },
        { ...seeded[0], id: "1", paused: true, pinned: false },
      ],
    })
    expect(
      screen.getByRole("button", { name: "Filter NOT host.name: canary-*" })
    ).toHaveAttribute("data-negate", "true")
    expect(
      screen.getByRole("button", { name: "Filter NOT host.name: canary-*" })
    ).toHaveAttribute("data-pinned", "true")
    expect(
      screen.getByRole("button", { name: "Filter service.name: checkout" })
    ).toHaveAttribute("data-paused", "true")
  })

  it("excludes a pill by flipping negate in the address", async () => {
    const { user, update, run } = setup()
    await user.click(
      screen.getByRole("button", { name: "Filter service.name: checkout" })
    )
    await user.click(screen.getByRole("button", { name: "Exclude" }))
    expect(lastFilters(update)[0]).toMatchObject({
      field: "service.name",
      negate: true,
    })
    expect(run).toHaveBeenCalledWith("timeout")
  })

  it("offers Include on an excluded pill", async () => {
    const { user, update } = setup()
    await user.click(
      screen.getByRole("button", { name: "Filter NOT host.name: canary-*" })
    )
    await user.click(screen.getByRole("button", { name: "Include" }))
    expect(lastFilters(update)[1].negate).toBe(false)
  })

  it("pauses a pill without dropping it", async () => {
    const { user, update } = setup()
    await user.click(
      screen.getByRole("button", { name: "Filter service.name: checkout" })
    )
    await user.click(screen.getByRole("button", { name: "Pause" }))
    const written = lastFilters(update)
    expect(written).toHaveLength(3)
    expect(written[0].paused).toBe(true)
    expect(applyPills("timeout", "elasticsearch", written)).not.toContain(
      "service.name"
    )
  })

  it("pins and deletes a pill", async () => {
    const { user, update } = setup()
    await user.click(
      screen.getByRole("button", { name: "Filter service.name: checkout" })
    )
    await user.click(screen.getByRole("button", { name: "Pin" }))
    expect(lastFilters(update)[0].pinned).toBe(true)

    await user.click(
      screen.getByRole("button", {
        name: "Filter kubernetes.namespace: staging",
      })
    )
    await user.click(screen.getByRole("button", { name: "Delete" }))
    expect(lastFilters(update).map((pill) => pill.field)).toEqual([
      "service.name",
      "host.name",
    ])
  })

  it("clears the address key when the last pill is deleted", async () => {
    const { user, update } = setup({ pills: [seeded[0]] })
    await user.click(
      screen.getByRole("button", { name: "Filter service.name: checkout" })
    )
    await user.click(screen.getByRole("button", { name: "Delete" }))
    expect(update).toHaveBeenLastCalledWith({ f: undefined })
  })

  it("edits a pill through the form", async () => {
    const { user, update } = setup()
    await user.click(
      screen.getByRole("button", { name: "Filter service.name: checkout" })
    )
    await user.click(screen.getByRole("button", { name: "Edit" }))
    const value = screen.getByRole("combobox", { name: "Value" })
    await user.clear(value)
    await user.type(value, "payments")
    await user.tab()
    await user.click(screen.getByRole("button", { name: "Save" }))
    expect(lastFilters(update).map((pill) => pill.value)).toEqual([
      "canary-*",
      "staging",
      "payments",
    ])
  })

  it("adds a pill from the form", async () => {
    const { user, update } = setup({ pills: [] })
    await user.click(screen.getByRole("button", { name: "Add filter" }))
    const dialog = screen.getByRole("dialog")
    await user.type(
      within(dialog).getByRole("combobox", { name: "Field" }),
      "log.level"
    )
    await user.click(await screen.findByRole("option", { name: "log.level" }))
    await user.click(within(dialog).getByRole("combobox", { name: "Operator" }))
    await user.click(await screen.findByRole("option", { name: "is not" }))
    await user.type(
      within(dialog).getByRole("combobox", { name: "Value" }),
      "debug"
    )
    expect(within(dialog).getByText("3 values in the range")).toBeVisible()
    await user.tab()
    await user.click(within(dialog).getByRole("button", { name: "Add filter" }))
    expect(lastFilters(update)).toEqual([
      {
        id: "0",
        kind: "is",
        field: "log.level",
        value: "debug",
        negate: true,
        pinned: false,
        paused: false,
      },
    ])
  })

  it("offers only is on Loki", async () => {
    const { user } = setup({ type: "loki", pills: [] })
    await user.click(screen.getByRole("button", { name: "Add filter" }))
    await user.click(screen.getByRole("combobox", { name: "Operator" }))
    expect(
      screen.getAllByRole("option").map((option) => option.textContent)
    ).toEqual(["is", "is not"])
  })

  it("says when a pill cannot be applied on this datasource", () => {
    setup({
      type: "loki",
      q: '{service_name="checkout"}',
      pills: [{ ...seeded[0] }, { ...seeded[2], kind: "exists", id: "1" }],
    })
    expect(screen.getAllByText("not applied on Loki")).toHaveLength(1)
  })

  it("keeps only pinned pills when the query text changes", () => {
    const { changeQuery, update, announce } = setup()
    changeQuery("payment failed")
    expect(lastFilters(update).map((pill) => pill.field)).toEqual(["host.name"])
    expect(update).toHaveBeenLastCalledWith(expect.anything(), {
      replace: true,
    })
    expect(announce).toHaveBeenCalledWith(
      "2 filters cleared; pinned filters kept"
    )
  })

  it("keeps every pill when the first query is run", () => {
    const { changeQuery, update } = setup({ q: undefined })
    changeQuery("timeout")
    expect(update).not.toHaveBeenCalled()
  })

  it("does not run before a query exists", async () => {
    const { user, run, update } = setup({ q: undefined })
    await user.click(
      screen.getByRole("button", { name: "Filter service.name: checkout" })
    )
    await user.click(screen.getByRole("button", { name: "Pause" }))
    expect(update).toHaveBeenCalled()
    expect(run).not.toHaveBeenCalled()
  })
})
