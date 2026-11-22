import { act, screen, waitFor, within } from "@testing-library/react"
import { useEffect, useState } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AuditLog } from "./audit-log"
import type { AuditView } from "./search"
import { type AuditEvent, liveRowLimit } from "./stream"
import { FakeStream, refusedStream } from "./stream-fake"

const fetchMock = vi.hoisted(() => vi.fn())
const toastSuccess = vi.hoisted(() => vi.fn())

vi.mock("sonner", () => ({ toast: { success: toastSuccess } }))

let liveStream: FakeStream | null = null
let streamRequests: { url: string; init?: RequestInit }[] = []

function auditEvent(id: string, fields: Partial<AuditEvent> = {}): AuditEvent {
  return {
    id,
    seq: 1,
    occurred_at: "2026-11-21T06:00:00Z",
    category: "content",
    action: "dashboard.update",
    outcome: "success",
    reason: null,
    actor_kind: "user",
    actor_name: "Ada",
    initiator: "person",
    origin: "web",
    origin_client: null,
    target_name: "Checkout",
    target_id: null,
    data: {},
    ...fields,
  } as AuditEvent
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  })
}

type Routes = {
  total?: number
  events?: (url: URL) => Response
  stream?: () => Response
}

function serve(routes: Routes = {}) {
  fetchMock.mockImplementation(
    (request: Request | string, init?: RequestInit) => {
      const url = new URL(
        typeof request === "string" ? request : request.url,
        "http://localhost"
      )
      switch (url.pathname) {
        case "/api/v1/audit/stream": {
          streamRequests.push({ url: String(request), init })
          if (routes.stream) return Promise.resolve(routes.stream())
          liveStream = new FakeStream()
          return Promise.resolve(liveStream.response)
        }
        case "/api/v1/audit/events":
          return Promise.resolve(
            routes.events?.(url) ??
              json({ events: [], next_cursor: null, form: "kql" })
          )
        case "/api/v1/audit/facets":
          return Promise.resolve(
            json({
              field: url.searchParams.get("field"),
              values:
                url.searchParams.get("field") === "outcome"
                  ? [
                      { value: "success", count: 40 },
                      { value: "denied", count: 2 },
                    ]
                  : [],
              form: "kql",
            })
          )
        case "/api/v1/audit/histogram":
          return Promise.resolve(
            json({
              step_ms: 1_800_000,
              total: routes.total ?? 42,
              buckets: [
                { time: "2026-11-21T05:00:00Z", count: 40 },
                { time: "2026-11-21T05:30:00Z", count: 2 },
              ],
              form: "kql",
            })
          )
        case "/api/v1/audit/fields":
          return Promise.resolve(
            json({
              fields: [
                {
                  name: "outcome",
                  alias: null,
                  type: "enum",
                  description: "",
                  values: [],
                },
              ],
            })
          )
        default:
          return Promise.resolve(json({ error: "no", code: "not_found" }, 404))
      }
    }
  )
}

const defaults: AuditView = { q: "", from: "now-24h", to: "now", event: null }

const outside: { setView: (view: AuditView) => void } = { setView: () => {} }

function MovingView({ initial }: { initial: AuditView }) {
  const [view, setView] = useState(initial)
  useEffect(() => {
    outside.setView = setView
  }, [])
  return <AuditLog view={view} onView={vi.fn()} />
}

function renderPage(view: Partial<AuditView> = {}) {
  const onView = vi.fn()
  const { user } = renderWithProviders(
    <AuditLog view={{ ...defaults, ...view }} onView={onView} />,
    { route: { path: "/", url: "/" } }
  )
  return { onView, user }
}

const hour = 3_600_000

function shiftClock(): (by: number) => void {
  const real = Date.now.bind(Date)
  let shift = 0
  vi.spyOn(Date, "now").mockImplementation(() => real() + shift)
  return (by) => {
    shift += by
  }
}

function listRequests(): URL[] {
  return fetchMock.mock.calls
    .map(([request]) => {
      const raw = typeof request === "string" ? request : request.url
      return new URL(raw, "http://localhost")
    })
    .filter((url) => url.pathname === "/api/v1/audit/events")
}

function untilOf(url: URL): number {
  return Date.parse(url.searchParams.get("until") ?? "")
}

beforeEach(() => {
  fetchMock.mockReset()
  toastSuccess.mockReset()
  liveStream = null
  streamRequests = []
  vi.stubGlobal("fetch", fetchMock)
  return () => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  }
})

describe("AuditLog", () => {
  it("opens Logs on the audit datasource with the applied query and range", async () => {
    serve()
    renderPage({ q: "origin:mcp", from: "now-7d" })
    const link = await screen.findByRole("link", { name: "Open in Logs" })
    const href = new URL(link.getAttribute("href") ?? "", "http://localhost")
    expect(href.pathname).toBe("/logs/slashobserve-audit")
    expect(Object.fromEntries(href.searchParams)).toEqual({
      q: "origin:mcp",
      from: "now-7d",
      to: "now",
    })
  })

  it("lists events newest first and loads older ones with the cursor", async () => {
    serve({
      events: (url) =>
        url.searchParams.get("cursor") === "next"
          ? json({
              events: [auditEvent("e3", { action: "auth.login" })],
              next_cursor: null,
              form: "kql",
            })
          : json({
              events: [
                auditEvent("e1", { action: "dashboard.delete" }),
                auditEvent("e2", { outcome: "denied", actor_name: "Zoë" }),
              ],
              next_cursor: "next",
              form: "kql",
            }),
    })
    const { user } = renderPage()
    const table = await screen.findByRole("region", { name: "Audit events" })
    const rows = within(table).getAllByRole("row").slice(1)
    expect(rows.map((row) => row.textContent)).toEqual([
      expect.stringContaining("dashboard.delete"),
      expect.stringContaining("Zoë"),
    ])
    expect(within(rows[1]).getByText("Denied")).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Load older events" }))
    expect(await within(table).findByText("auth.login")).toBeInTheDocument()
    const first = fetchMock.mock.calls
      .map(([request]) => new URL((request as Request).url))
      .find((url) => url.pathname === "/api/v1/audit/events")
    expect(first?.searchParams.get("since")).toMatch(/Z$/)
    expect(first?.searchParams.get("until")).toMatch(/Z$/)
  })

  it("marks the part of the query the server could not read", async () => {
    serve({
      events: () =>
        json(
          {
            error: 'unexpected ")"',
            code: "query_invalid",
            start: 13,
            end: 14,
          },
          422
        ),
    })
    renderPage({ q: 'actor:"Zoë" )' })
    expect(await screen.findByText('unexpected ")"')).toBeInTheDocument()
    expect(screen.getByRole("alert").querySelector("mark")).toHaveTextContent(
      ")"
    )
    expect(
      screen.queryByRole("region", { name: "Audit events" })
    ).not.toBeInTheDocument()
  })

  it("adds a facet value to the query", async () => {
    serve()
    const { onView, user } = renderPage({ q: "origin:mcp" })
    const outcomes = await screen.findByRole("group", {
      name: "Outcome values",
    })
    await user.click(
      await within(outcomes).findByRole("button", { name: "denied 2" })
    )
    await waitFor(() =>
      expect(onView).toHaveBeenCalledWith({
        ...defaults,
        q: 'origin:mcp and outcome:"denied"',
      })
    )
    expect(listRequests()).toHaveLength(2)
  })

  it("never sends or writes a trailing newline of the query", async () => {
    serve()
    const { onView, user } = renderPage({ q: "origin:mcp\n" })
    await screen.findByText("No audit events match these filters.")
    await user.click(screen.getByRole("button", { name: /^Search/ }))
    await waitFor(() =>
      expect(onView).toHaveBeenCalledWith({ ...defaults, q: "origin:mcp" })
    )
    expect(listRequests().at(-1)?.searchParams.get("q")).toBe("origin:mcp")
  })

  it("stops dimming the results when the address moves on", async () => {
    serve({
      events: (url) =>
        (url.searchParams.get("q") ?? "").includes("denied")
          ? json(
              { error: "cannot read this query", code: "query_invalid" },
              422
            )
          : json({
              events: [auditEvent("e1", { action: "dashboard.delete" })],
              next_cursor: null,
              form: "kql",
            }),
    })
    const { user } = renderWithProviders(
      <MovingView initial={{ ...defaults, q: "origin:mcp" }} />,
      { route: { path: "/", url: "/" } }
    )
    const outcomes = await screen.findByRole("group", {
      name: "Outcome values",
    })
    await user.click(
      await within(outcomes).findByRole("button", { name: "denied 2" })
    )
    await screen.findByText("cannot read this query")
    const dimmed = () =>
      screen
        .getByRole("region", { name: "Audit events" })
        .closest(".opacity-60")
    expect(dimmed()).not.toBeNull()
    act(() => outside.setView({ ...defaults, q: "origin:cli" }))
    await waitFor(() => expect(dimmed()).toBeNull())
    expect(screen.queryByText("cannot read this query")).toBeNull()
  })

  it("keeps the last good results and the address when the server rejects a query", async () => {
    serve({
      events: (url) =>
        (url.searchParams.get("q") ?? "").includes("denied")
          ? json(
              { error: "cannot read this query", code: "query_invalid" },
              422
            )
          : json({
              events: [auditEvent("e1", { action: "dashboard.delete" })],
              next_cursor: null,
              form: "kql",
            }),
    })
    const { onView, user } = renderPage({ q: "origin:mcp" })
    const outcomes = await screen.findByRole("group", {
      name: "Outcome values",
    })
    await user.click(
      await within(outcomes).findByRole("button", { name: "denied 2" })
    )
    expect(
      await screen.findByText("cannot read this query")
    ).toBeInTheDocument()
    expect(
      screen.getByRole("region", { name: "Audit events" })
    ).toHaveTextContent("dashboard.delete")
    expect(screen.getByText(/42 events/)).toBeInTheDocument()
    expect(onView).not.toHaveBeenCalled()
  })

  it("offers to clear filters when nothing matches", async () => {
    serve()
    const { onView, user } = renderPage({
      q: "outcome:failed",
      from: "now-7d",
    })
    const clear = await screen.findByRole("button", { name: "Clear filters" })
    expect(
      screen.getByText("No audit events match these filters.")
    ).toBeInTheDocument()
    await user.click(clear)
    expect(onView).toHaveBeenCalledWith(defaults)
  })

  it("refuses a time range longer than 366 days without asking the server", async () => {
    serve()
    renderPage({ from: "now-400d" })
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The audit log searches at most 366 days at a time."
    )
    expect(
      fetchMock.mock.calls.some(([request]) =>
        (request as Request).url.includes("/audit/events")
      )
    ).toBe(false)
  })

  it("keeps Live off for a range that does not end now", async () => {
    serve()
    renderPage({ from: "now-2d", to: "now-1d" })
    expect(await screen.findByRole("switch")).toHaveAttribute(
      "aria-disabled",
      "true"
    )
    expect(
      screen.getByText("Live follows a time range that ends now.")
    ).toBeInTheDocument()
  })

  it("puts live events on top and turns Live off when the stream refuses", async () => {
    serve({
      events: () =>
        json({
          events: [auditEvent("e1", { action: "dashboard.delete" })],
          next_cursor: null,
          form: "kql",
        }),
    })
    const { user } = renderPage({ q: "origin:mcp" })
    const table = await screen.findByRole("region", { name: "Audit events" })
    await user.click(screen.getByRole("switch"))
    await waitFor(() => expect(liveStream).not.toBe(null))
    const stream = liveStream as unknown as FakeStream
    expect(streamRequests[0].url).toBe("/api/v1/audit/stream?q=origin%3Amcp")
    act(() => {
      stream.frame("ready", {})
      stream.frame("audit", auditEvent("e9", { action: "auth.login" }), "x-1")
      stream.frame(
        "audit",
        auditEvent("e1", { action: "dashboard.delete" }),
        "x-2"
      )
    })
    await within(table).findByText("auth.login")
    expect(await screen.findByText(/43 events/)).toBeInTheDocument()
    expect(
      within(screen.getByRole("group", { name: "Outcome values" })).getByRole(
        "button",
        { name: "success 41" }
      )
    ).toBeInTheDocument()
    const rows = within(table).getAllByRole("row").slice(1)
    expect(rows.map((row) => row.textContent)).toEqual([
      expect.stringContaining("auth.login"),
      expect.stringContaining("dashboard.delete"),
    ])
    act(() => stream.frame("forbidden", { code: "forbidden_role" }))
    expect(
      await screen.findByText(
        "Live stopped: only Admins can follow the audit log."
      )
    ).toBeInTheDocument()
    expect(screen.getByRole("switch")).toHaveAttribute("aria-checked", "false")
    expect(streamRequests[0].init?.signal?.aborted).toBe(true)
  })

  it("reloads the list up to now when Live turns on", async () => {
    serve()
    const advance = shiftClock()
    const { user } = renderPage()
    await screen.findByText("No audit events in the last 24 hours.")
    const opened = untilOf(listRequests()[0])
    advance(hour)
    await user.click(screen.getByRole("switch"))
    await waitFor(() => expect(listRequests()).toHaveLength(2))
    expect(untilOf(listRequests()[1]) - opened).toBeGreaterThanOrEqual(hour)
  })

  it("reloads the list up to now when the view changes from outside", async () => {
    serve()
    const advance = shiftClock()
    renderWithProviders(<MovingView initial={defaults} />, {
      route: { path: "/", url: "/" },
    })
    await screen.findByText("No audit events in the last 24 hours.")
    const opened = untilOf(listRequests()[0])
    advance(hour)
    act(() => outside.setView({ ...defaults, q: "origin:mcp" }))
    await waitFor(() => expect(listRequests()).toHaveLength(2))
    const [, back] = listRequests()
    expect(back.searchParams.get("q")).toBe("origin:mcp")
    expect(untilOf(back) - opened).toBeGreaterThanOrEqual(hour)
  })

  it("folds the live rows into the list once they pass the live row limit", async () => {
    let lists = 0
    serve({
      events: () => {
        lists++
        return json({
          events: lists < 3 ? [] : [auditEvent("reloaded")],
          next_cursor: null,
          form: "kql",
        })
      },
    })
    const { user } = renderPage()
    await screen.findByText("No audit events in the last 24 hours.")
    await user.click(screen.getByRole("switch"))
    await waitFor(() => expect(liveStream).not.toBe(null))
    const stream = liveStream as unknown as FakeStream
    act(() => {
      stream.frame("ready", {})
      for (let index = 0; index <= liveRowLimit; index++) {
        stream.frame(
          "audit",
          auditEvent(`live-${index}`, { occurred_at: "2026-01-01T00:00:00Z" }),
          `x-${index}`
        )
      }
    })
    await waitFor(() => expect(lists).toBe(3))
    const table = await screen.findByRole("region", { name: "Audit events" })
    await waitFor(() =>
      expect(within(table).getAllByRole("row").slice(1)).toHaveLength(1)
    )
  })

  it.each([
    [1_000_001, "1,000,001 events match; the export stops at 1,000,000."],
    [1_000_000, null],
  ])(
    "exports %i matching events as a browser download",
    async (total, warning) => {
      serve({ total })
      const clicked: string[] = []
      vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(
        function (this: HTMLAnchorElement) {
          clicked.push(this.href)
        }
      )
      const { user } = renderPage({ q: "origin:mcp" })
      await screen.findByText("No audit events match these filters.")
      await user.click(screen.getByRole("button", { name: "Export" }))
      expect(await screen.findByRole("menu")).toHaveClass("w-max")
      await user.click(await screen.findByRole("menuitem", { name: /^CSV/ }))
      await waitFor(() => expect(clicked).toHaveLength(1))
      expect(toastSuccess).toHaveBeenCalledWith(
        "Download started: the audit log as CSV."
      )
      const url = new URL(clicked[0])
      expect(url.pathname).toBe("/api/v1/audit/export")
      expect(url.searchParams.get("q")).toBe("origin:mcp")
      expect(url.searchParams.get("format")).toBe("csv")
      expect(
        fetchMock.mock.calls.some(([request]) =>
          String(typeof request === "string" ? request : request.url).includes(
            "/audit/export"
          )
        )
      ).toBe(false)
      if (warning === null) {
        expect(screen.queryByText(/the export stops at/)).toBe(null)
      } else {
        expect(await screen.findByText(warning)).toBeInTheDocument()
      }
    }
  )

  it("says Live is off when the caller already holds five streams", async () => {
    serve({ stream: () => refusedStream(429, "too_many_streams") })
    const { user } = renderPage()
    await screen.findByText("No audit events in the last 24 hours.")
    await user.click(screen.getByRole("switch"))
    expect(
      await screen.findByText(/already follow 5 audit streams/)
    ).toBeInTheDocument()
    expect(screen.getByRole("switch")).toHaveAttribute("aria-checked", "false")
  })

  it("restarts Live with a note and reloads the list when a resume is refused", async () => {
    const first = new FakeStream()
    const fresh = new FakeStream()
    const answers = [
      first.response,
      refusedStream(400, "invalid_request"),
      fresh.response,
    ]
    let lists = 0
    serve({
      stream: () => answers.shift() as Response,
      events: () => {
        lists++
        return json({ events: [], next_cursor: null, form: "kql" })
      },
    })
    vi.spyOn(Math, "random").mockReturnValue(0)
    const { user } = renderPage()
    await screen.findByText("No audit events in the last 24 hours.")
    await user.click(screen.getByRole("switch"))
    await waitFor(() => expect(streamRequests).toHaveLength(1))
    act(() => first.frame("audit", auditEvent("e9"), "x-9"))
    await screen.findAllByText("dashboard.update")
    act(() => first.drop())
    await waitFor(() => expect(streamRequests).toHaveLength(3), {
      timeout: 3000,
    })
    act(() => fresh.frame("ready", {}))
    expect(await screen.findByText(/Live follow restarted/)).toBeInTheDocument()
    await waitFor(() => expect(lists).toBe(3))
    expect(screen.getByRole("switch")).toHaveAttribute("aria-checked", "true")
  })
})
