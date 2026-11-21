import { act, screen, waitFor, within } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AuditSettings } from "./audit-settings"
import type { AuditView } from "./search"
import type { AuditEvent } from "./stream"
import { FakeStream, refusedStream } from "./stream-fake"

const fetchMock = vi.hoisted(() => vi.fn())

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
              total: 42,
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

function renderPage(view: Partial<AuditView> = {}) {
  const onView = vi.fn()
  const { user } = renderWithProviders(
    <AuditSettings view={{ ...defaults, ...view }} onView={onView} />
  )
  return { onView, user }
}

beforeEach(() => {
  fetchMock.mockReset()
  liveStream = null
  streamRequests = []
  vi.stubGlobal("fetch", fetchMock)
  return () => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  }
})

describe("AuditSettings", () => {
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
    expect(onView).toHaveBeenCalledWith({
      ...defaults,
      q: 'origin:mcp and outcome:"denied"',
    })
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

  it("refuses a time range longer than 366 days without asking the server", () => {
    serve()
    renderPage({ from: "now-400d" })
    expect(screen.getByRole("alert")).toHaveTextContent(
      "The audit log searches at most 366 days at a time."
    )
    expect(
      fetchMock.mock.calls.some(([request]) =>
        (request as Request).url.includes("/audit/events")
      )
    ).toBe(false)
  })

  it("keeps Live off for a range that does not end now", () => {
    serve()
    renderPage({ from: "now-2d", to: "now-1d" })
    expect(screen.getByRole("switch")).toHaveAttribute("aria-disabled", "true")
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
    await screen.findByText("dashboard.update")
    act(() => first.drop())
    await waitFor(() => expect(streamRequests).toHaveLength(3), {
      timeout: 3000,
    })
    act(() => fresh.frame("ready", {}))
    expect(await screen.findByText(/Live follow restarted/)).toBeInTheDocument()
    await waitFor(() => expect(lists).toBe(2))
    expect(screen.getByRole("switch")).toHaveAttribute("aria-checked", "true")
  })
})
