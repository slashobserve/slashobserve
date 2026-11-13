import { screen, within } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { datasourceKeys, exploreKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { FieldsAside } from "./fields-aside"
import type { LogFacets } from "./facets"
import type { LogsSearch } from "./search"
import { LogsWorkspaceContext, type LogsWorkspace } from "./workspace"

const range = { from: "now-1h", to: "now" }

const esFacets: LogFacets = {
  sampled: false,
  lines_sampled: 0,
  truncated: false,
  fields: [
    {
      name: "log.level",
      kind: "keyword",
      exact: true,
      cardinality: 2,
      truncated: false,
      values: [
        { value: "error", count: 1412 },
        { value: "warn", count: 1040 },
      ],
    },
    {
      name: "http.response.status_code",
      kind: "number",
      exact: true,
      cardinality: 1,
      truncated: false,
      values: [],
    },
  ],
}

const lokiFacets: LogFacets = {
  sampled: true,
  lines_sampled: 1000,
  truncated: false,
  fields: [
    {
      name: "service_name",
      kind: "label",
      exact: true,
      unit: "bytes",
      cardinality: 1,
      truncated: false,
      values: [{ value: "checkout", count: 1572864 }],
    },
    {
      name: "status",
      kind: "field",
      exact: false,
      cardinality: 2,
      truncated: false,
      values: [{ value: "500", count: 30 }],
    },
    {
      name: "trace_id",
      kind: "metadata",
      exact: false,
      cardinality: 1,
      truncated: false,
      values: [{ value: "abc", count: 9 }],
    },
  ],
}

function setup({
  type = "elasticsearch",
  search = { q: "service.name:checkout" },
  names = ["log.level", "message", "http.response.status_code"],
  facets = esFacets,
  effectiveQuery = search.q,
}: {
  type?: string
  search?: LogsSearch
  effectiveQuery?: string
  names?: string[]
  facets?: LogFacets
} = {}) {
  const run = vi.fn()
  const setDraft = vi.fn()
  const workspace = {
    datasource: {
      uid: "ds",
      type,
      name: "ds",
      index: "logs-*",
      healthy: true,
      checked_at: "2026-11-07T12:00:00Z",
      dashboard_count: 0,
    },
    search,
    effectiveQuery,
    range,
    run,
    setDraft,
  } as unknown as LogsWorkspace
  const queryClient = testQueryClient()
  queryClient.setQueryData(datasourceKeys.labels("ds", range), {
    labels: names,
    truncated: false,
  })
  if (effectiveQuery !== undefined) {
    queryClient.setQueryData(
      exploreKeys.facets("ds", { query: effectiveQuery, ...range, limit: 5 }),
      facets
    )
  }
  const view = renderWithProviders(
    <LogsWorkspaceContext value={workspace}>
      <FieldsAside />
    </LogsWorkspaceContext>,
    { queryClient }
  )
  return { ...view, run, setDraft }
}

describe("FieldsAside", () => {
  beforeEach(() => localStorage.clear())

  it("lists every mapped field, counted or not, under the mapping note", () => {
    setup()
    const aside = screen.getByRole("complementary", { name: "Fields" })
    expect(
      within(aside).getByText("From the logs-* mapping, 3 fields")
    ).toBeVisible()
    const available = within(aside).getByRole("region", {
      name: "Available fields",
    })
    expect(
      within(available)
        .getAllByRole("button", { expanded: false })
        .map((button) => button.textContent)
    ).toEqual(["#http.response.status_code", "klog.level", "message"])
  })

  it("counts values over the query with the filter pills applied", async () => {
    const { user } = setup({
      search: { q: "timeout" },
      effectiveQuery: "timeout AND service.name:checkout",
    })
    await user.click(screen.getByRole("button", { name: /log\.level/ }))
    expect(
      screen.getByRole("group", { name: "Top values of log.level" })
    ).toBeVisible()
  })

  it("filters the field list by name", async () => {
    const { user } = setup()
    await user.type(
      screen.getByRole("textbox", { name: "Search field names" }),
      "LEVEL"
    )
    expect(screen.getByRole("button", { name: /log\.level/ })).toBeVisible()
    expect(screen.queryByRole("button", { name: /message/ })).toBeNull()
  })

  it("shows the chosen columns apart from the rest", () => {
    setup({ search: { q: "a", cols: "message" } })
    const selected = screen.getByRole("region", { name: "Selected fields" })
    expect(
      within(selected).getByRole("button", { name: /message/ })
    ).toBeVisible()
    const available = screen.getByRole("region", { name: "Available fields" })
    expect(
      within(available).queryByRole("button", { name: /message/ })
    ).toBeNull()
  })

  it("expands a field to its top values with counts and how they were counted", async () => {
    const { user } = setup()
    await user.click(screen.getByRole("button", { name: /log\.level/ }))
    const values = screen.getByRole("group", {
      name: "Top values of log.level",
    })
    expect(within(values).getByText("Top 2 values")).toBeVisible()
    expect(within(values).getByText("exact")).toBeVisible()
    expect(
      within(values).getByRole("group", { name: "error, 1,412 lines" })
    ).toBeVisible()
    expect(
      within(values).getByText("Counted over every matching line.")
    ).toBeVisible()
  })

  it("says so when a field has no values in the range", async () => {
    const { user } = setup()
    await user.click(screen.getByRole("button", { name: /status_code/ }))
    expect(screen.getByText("No values in this range.")).toBeVisible()
  })

  it("filters the query in with one click on a value", async () => {
    const { user, run, setDraft } = setup()
    await user.click(screen.getByRole("button", { name: /log\.level/ }))
    await user.click(
      screen.getByRole("button", { name: "Include log.level error" })
    )
    const next = 'service.name:checkout and log.level:"error"'
    expect(setDraft).toHaveBeenCalledWith(next)
    expect(run).toHaveBeenCalledWith(next)
  })

  it("filters the query out with the exclude button", async () => {
    const { user, run } = setup()
    await user.click(screen.getByRole("button", { name: /log\.level/ }))
    await user.click(
      screen.getByRole("button", { name: "Exclude log.level warn" })
    )
    expect(run).toHaveBeenCalledWith(
      'service.name:checkout and not log.level:"warn"'
    )
  })

  it("disables the buttons when the query cannot take the filter", async () => {
    const { user } = setup({ search: { q: "lucene: level:error" } })
    await user.click(screen.getByRole("button", { name: /log\.level/ }))
    expect(
      screen.getByRole("button", { name: "Include log.level error" })
    ).toBeDisabled()
    expect(
      screen.getByRole("button", { name: "Exclude log.level error" })
    ).toBeDisabled()
  })

  it("counts Loki index labels in bytes and the parsed fields from the sample", async () => {
    const { user, run } = setup({
      type: "loki",
      search: { q: '{service_name="checkout"}' },
      names: ["service_name"],
      facets: lokiFacets,
    })
    expect(screen.getByText("From the stream labels, 3 fields")).toBeVisible()
    await user.click(screen.getByRole("button", { name: /service_name/ }))
    expect(
      screen.getByRole("group", { name: "checkout, 1.5 MiB" })
    ).toBeVisible()
    await user.click(screen.getByRole("button", { name: /status/ }))
    expect(screen.getByText("sampled")).toBeVisible()
    expect(
      screen.getByText("Counted over the newest 1,000 lines.")
    ).toBeVisible()
    await user.click(screen.getByRole("button", { name: "Include status 500" }))
    expect(run).toHaveBeenCalledWith('{service_name="checkout"} | status="500"')
  })

  it("asks for a query before showing values", async () => {
    const { user } = setup({ search: {} })
    await user.click(screen.getByRole("button", { name: /log\.level/ }))
    expect(screen.getByText("Run a query first.")).toBeVisible()
  })

  it("folds to a strip and unfolds again, remembering the choice", async () => {
    const { user } = setup()
    await user.click(screen.getByRole("button", { name: "Hide fields" }))
    expect(screen.queryByRole("complementary", { name: "Fields" })).toBeNull()
    expect(
      screen.getByRole("complementary", { name: "Fields, hidden" })
    ).toBeVisible()
    expect(localStorage.getItem("slashobserve-logs-facets-open")).toBe("false")
    await user.click(screen.getByRole("button", { name: "Show fields" }))
    expect(screen.getByRole("complementary", { name: "Fields" })).toBeVisible()
    expect(localStorage.getItem("slashobserve-logs-facets-open")).toBe("true")
  })

  it("opens folded when the person left it folded", () => {
    localStorage.setItem("slashobserve-logs-facets-open", "false")
    setup()
    expect(
      screen.getByRole("complementary", { name: "Fields, hidden" })
    ).toBeVisible()
  })
})
