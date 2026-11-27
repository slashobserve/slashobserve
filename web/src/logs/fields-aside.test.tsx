import { fireEvent, screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/api/errors"
import { datasourceKeys, exploreKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { FieldsAside, FieldsDrawer } from "./fields-aside"
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
  openLine = false,
  drawer = false,
  linesLoading = false,
  linesRerunning = false,
  linesTimedOut = false,
  seedFacets = true,
  window = range,
}: {
  window?: { from: string; to: string }
  seedFacets?: boolean
  linesTimedOut?: boolean
  linesLoading?: boolean
  linesRerunning?: boolean
  drawer?: boolean
  openLine?: boolean
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
    range: window,
    run,
    setDraft,
    results: {
      isLoading: linesLoading,
      isFetching: linesLoading || linesRerunning,
      isPlaceholderData: linesRerunning,
      isSuccess: !linesLoading && !linesTimedOut,
      isError: linesTimedOut,
      error: linesTimedOut
        ? new ApiError(504, "query_timeout", "query timed out after 15s")
        : null,
    },
    tail: { shown: false },
    detail: { selection: openLine ? {} : null },
  } as unknown as LogsWorkspace
  const queryClient = testQueryClient()
  queryClient.setQueryData(datasourceKeys.labels("ds", window), {
    labels: names,
    truncated: false,
  })
  if (effectiveQuery !== undefined && seedFacets) {
    queryClient.setQueryData(
      exploreKeys.facets("ds", { query: effectiveQuery, ...window, limit: 5 }),
      facets
    )
  }
  const view = renderWithProviders(
    <LogsWorkspaceContext value={workspace}>
      <FieldsAside />
      {drawer ? <FieldsDrawer /> : null}
    </LogsWorkspaceContext>,
    { queryClient }
  )
  return { ...view, run, setDraft, queryClient }
}

function stubNarrow() {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: true,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  }))
}

describe("FieldsAside", () => {
  beforeEach(() => localStorage.clear())
  afterEach(() => vi.unstubAllGlobals())

  it("moves the field list into a drawer when the page is too narrow for the side panel", async () => {
    stubNarrow()
    const { user, run } = setup({ drawer: true })
    expect(screen.queryByRole("complementary")).toBeNull()
    await user.click(screen.getByRole("button", { name: "Fields" }))
    const drawer = await screen.findByRole("dialog", { name: "Fields" })
    await user.click(within(drawer).getByRole("button", { name: /log\.level/ }))
    await user.click(
      within(drawer).getByRole("button", { name: "Include log.level error" })
    )
    expect(run).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole("dialog", { name: "Fields" })).toBeNull()
  })

  it("counts the values only once the lines have arrived", async () => {
    const fetcher = vi.fn(async () => Response.json(esFacets))
    vi.stubGlobal("fetch", fetcher)
    setup({ linesLoading: true, seedFacets: false })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(fetcher).not.toHaveBeenCalled()
  })

  it("counts a wide range as soon as the lines are in, without waiting for the volume", async () => {
    const fetcher = vi.fn(async () => Response.json(esFacets))
    vi.stubGlobal("fetch", fetcher)
    setup({ window: { from: "now-7d", to: "now" }, seedFacets: false })
    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1))
  })

  it("counts a narrow range at once without waiting for the volume", async () => {
    const fetcher = vi.fn(async () => Response.json(esFacets))
    vi.stubGlobal("fetch", fetcher)
    setup({ seedFacets: false })
    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1))
  })

  it("does not count the values when the lines timed out, and shows the failure instead of waiting", async () => {
    const fetcher = vi.fn(async () => Response.json(esFacets))
    vi.stubGlobal("fetch", fetcher)
    setup({ linesTimedOut: true, seedFacets: false })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(fetcher).not.toHaveBeenCalled()
    expect(screen.getByText("The query failed, see the results.")).toBeVisible()
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull()
    expect(screen.queryByText(/when the query answers/)).toBeNull()
    expect(screen.queryByText("Available")).toBeNull()
  })

  it("counts the values only once a rerun's lines have arrived", async () => {
    const fetcher = vi.fn(async () => Response.json(esFacets))
    vi.stubGlobal("fetch", fetcher)
    setup({ linesRerunning: true, seedFacets: false })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(fetcher).not.toHaveBeenCalled()
  })

  it("offers no drawer while the side panel fits", () => {
    setup({ drawer: true })
    expect(screen.queryByRole("button", { name: "Fields" })).toBeNull()
  })

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

  it("folds while a line is open without forgetting the stored choice", async () => {
    const { user } = setup({ openLine: true })
    expect(
      screen.getByRole("complementary", { name: "Fields, hidden" })
    ).toBeVisible()
    expect(localStorage.getItem("slashobserve-logs-facets-open")).toBeNull()
    await user.click(screen.getByRole("button", { name: "Show fields" }))
    expect(screen.getByRole("complementary", { name: "Fields" })).toBeVisible()
  })

  it("widens with the keyboard on the resize handle and remembers the width", async () => {
    const { user } = setup()
    const aside = screen.getByRole("complementary", { name: "Fields" })
    expect(aside).toHaveStyle({ width: "248px" })
    const grip = screen.getByRole("separator", { name: "Resize fields" })
    grip.focus()
    await user.keyboard("{Shift>}{ArrowRight}{/Shift}")
    expect(aside).toHaveStyle({ width: "312px" })
    expect(localStorage.getItem("slashobserve-logs-fields-width")).toBe("312")
    await user.dblClick(grip)
    expect(aside).toHaveStyle({ width: "248px" })
    expect(localStorage.getItem("slashobserve-logs-fields-width")).toBe("248")
  })

  it("leaves the page selectable when it unmounts in the middle of a drag", async () => {
    const { unmount } = setup()
    const grip = screen.getByRole("separator", { name: "Resize fields" })
    fireEvent.pointerDown(grip, { button: 0, pointerId: 1, clientX: 100 })
    await waitFor(() => expect(document.body.style.userSelect).toBe("none"))
    unmount()
    expect(document.body.style.userSelect).toBe("")
    expect(document.body.style.cursor).toBe("")
  })

  it("opens at the width the person left it", () => {
    localStorage.setItem("slashobserve-logs-fields-width", "400")
    setup()
    expect(screen.getByRole("complementary", { name: "Fields" })).toHaveStyle({
      width: "400px",
    })
  })
})
