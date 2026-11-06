import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { DashboardSearch } from "@/dashboards/search"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { PublicPage } from "./public-page"
import { publicDashboardQuery, type PublicDashboard } from "./queries"
import type { PublicMode } from "./supported"

const token = `pub_${"a".repeat(43)}`

function publicBody(mode: PublicMode, panelTypes: string[]) {
  return {
    dashboard_id: "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01",
    title: "Checkout",
    version: 3,
    variables: mode,
    selections: { job: ["api"] },
    expires_at: null,
    datasources: [{ uid: "prom", type: "prometheus" }],
    spec: {
      specVersion: "0",
      title: "Checkout",
      time: { from: "now-1h", to: "now" },
      variables: [{ name: "job", type: "query", datasource: "prom" }],
      panels: panelTypes.map((type, index) => ({
        id: `p${index + 1}`,
        type,
        title: `Panel ${index + 1}`,
        gridPos: { x: 0, y: index * 4, w: 12, h: 4 },
        datasource: "prom",
        queries: [{ ref: "A" }],
      })),
    },
  }
}

const emptyData = {
  frames: [],
  meta: {
    kind: "instant",
    from: "2026-11-06T00:00:00Z",
    to: "2026-11-06T01:00:00Z",
    step_ms: 0,
    queries: [],
    cached: false,
    ran_at: "2026-11-06T01:00:00Z",
  },
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
}

function mockServer(body: ReturnType<typeof publicBody>) {
  const urls: URL[] = []
  const fetch = vi.fn((request: Request) => {
    const url = new URL(request.url)
    urls.push(url)
    if (url.pathname === `/api/v1/public/${token}`) {
      return Promise.resolve(json(body))
    }
    if (url.pathname.startsWith(`/api/v1/public/${token}/panels/`)) {
      return Promise.resolve(json(emptyData))
    }
    if (url.pathname === `/api/v1/public/${token}/variables/job/values`) {
      return Promise.resolve(
        json({ values: ["api", "web"], truncated: false, cached: false })
      )
    }
    return Promise.resolve(new Response(null, { status: 401 }))
  })
  vi.stubGlobal("fetch", fetch)
  return urls
}

class SeenAtOnce {
  private readonly callback: IntersectionObserverCallback
  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback
  }
  observe() {
    this.callback(
      [{ isIntersecting: true } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver
    )
  }
  unobserve() {}
  disconnect() {}
}

async function renderPublic(
  body: ReturnType<typeof publicBody>,
  search: DashboardSearch = { vars: {} }
) {
  const urls = mockServer(body)
  const queryClient = testQueryClient()
  const dashboard: PublicDashboard = await queryClient.fetchQuery(
    publicDashboardQuery(token)
  )
  renderWithProviders(
    <PublicPage
      token={token}
      dashboard={dashboard}
      search={search}
      onSearchChange={vi.fn()}
    />,
    { queryClient, route: { path: "/p/$token", url: `/p/${token}` } }
  )
  return urls
}

function panelRequests(urls: URL[]) {
  return urls.filter((url) => url.pathname.includes("/panels/"))
}

beforeEach(() => {
  vi.stubGlobal("IntersectionObserver", SeenAtOnce)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("PublicPage", () => {
  it("sends every request of a two-panel dashboard under /api/v1/public/", async () => {
    const urls = await renderPublic(publicBody("free", ["stat", "table"]))
    await waitFor(() => expect(panelRequests(urls)).toHaveLength(2))
    await waitFor(() => expect(screen.getAllByText("No data")).toHaveLength(2))
    expect(
      urls.some((url) => url.pathname.endsWith("/variables/job/values"))
    ).toBe(true)
    expect(
      urls.filter((url) => !url.pathname.startsWith("/api/v1/public/"))
    ).toEqual([])
  })

  it("offers a free link's query variable and starts from the stored selection", async () => {
    const urls = await renderPublic(publicBody("free", ["stat"]))
    expect(
      await screen.findByRole("combobox", { name: "job" })
    ).toHaveTextContent("api")
    await waitFor(() => expect(panelRequests(urls)).toHaveLength(1))
    expect(panelRequests(urls)[0].searchParams.getAll("var-job")).toEqual([
      "api",
    ])
  })

  it("shows a frozen link's selections as text and ignores the address", async () => {
    const urls = await renderPublic(publicBody("frozen", ["stat"]), {
      vars: { job: ["web"] },
    })
    expect(await screen.findByText("job: api")).toBeInTheDocument()
    expect(screen.queryByRole("combobox", { name: "job" })).toBeNull()
    await waitFor(() => expect(panelRequests(urls)).toHaveLength(1))
    expect(panelRequests(urls)[0].searchParams.getAll("var-job")).toEqual([
      "api",
    ])
  })

  it("renders panel types that need sign-in as unavailable without a request", async () => {
    const urls = await renderPublic(publicBody("free", ["dashlist", "stat"]))
    expect(
      await screen.findByText("Not available on a public link")
    ).toBeInTheDocument()
    await waitFor(() => expect(panelRequests(urls)).toHaveLength(1))
    expect(panelRequests(urls)[0].pathname).toBe(
      `/api/v1/public/${token}/panels/p2/data`
    )
  })
})
