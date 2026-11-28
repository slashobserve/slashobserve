import { cleanup, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { datasourceKeys } from "@/api/keys"
import { describeRange } from "@/lib/time"
import type { Block } from "@/observations/queries"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { PageRangeContext, type PageRange } from "./block-data"
import { BlockInspectContext, type BlockInspect } from "./block-inspect"
import { PanelBlock } from "./panel-block"

const person = {
  kind: "user",
  id: "u1",
  name: "Priya",
  origin: "web",
  client: null,
  run_id: null,
  label: "Priya",
}

function panelBlock(
  body: Record<string, unknown>,
  allowed: string[] = []
): Block {
  return {
    id: "b1",
    observation_id: "o1",
    version: 3,
    kind: "panel",
    body,
    allowed_actions: allowed,
    created_by: person,
  } as unknown as Block
}

const errorRate = {
  type: "timeseries",
  title: "Error rate",
  datasource: "prom-prod",
  queries: [{ query: "up" }],
}

const following = panelBlock({ panel: errorRate }, ["update"])

const noPage: PageRange = { from: null, to: null }

const readable = panelBlock({
  panel: {
    type: "timeseries",
    title: "Error rate",
    datasource: "prom-prod",
    queries: [{ query: "up" }],
  },
  from: "2026-11-06T10:00:00Z",
  to: "2026-11-06T11:00:00Z",
})

function json(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

const blockData = "/api/v1/blocks/b1/data"

function stubData(response: () => Response) {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    async (request) =>
      new URL(request.url).pathname === blockData
        ? response()
        : json({ available: false }, 200)
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

async function blockWrites(
  fetch: ReturnType<typeof stubData>
): Promise<unknown[]> {
  const writes = fetch.mock.calls
    .map(([request]) => request)
    .filter(
      (request) =>
        request.method === "PATCH" &&
        new URL(request.url).pathname === "/api/v1/blocks/b1"
    )
  return Promise.all(writes.map((request) => request.clone().json()))
}

function dataRequests(fetch: ReturnType<typeof stubData>): URL[] {
  return fetch.mock.calls
    .map(([request]) => new URL(request.url))
    .filter((url) => url.pathname === blockData)
}

function renderBlock(
  block: Block,
  inspect: BlockInspect | null = null,
  page: PageRange = noPage
) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(datasourceKeys.list(), {
    datasources: [
      {
        uid: "prom-prod",
        type: "prometheus",
        name: "Prometheus production",
        healthy: true,
        checked_at: "",
        dashboard_count: 0,
      },
    ],
  })
  return renderWithProviders(
    <PageRangeContext value={page}>
      <BlockInspectContext value={inspect}>
        <PanelBlock block={block} height={240} />
      </BlockInspectContext>
    </PageRangeContext>,
    {
      queryClient,
      route: { path: "/o/$observationId", url: "/o/o1" },
    }
  )
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

beforeEach(() => {
  vi.stubGlobal("IntersectionObserver", SeenAtOnce)
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe("PanelBlock", () => {
  it("asks the block data route for the block's own range", async () => {
    const fetch = stubData(() => json({ frames: [], meta: {} }, 200))
    renderBlock(readable)

    expect(await screen.findByText("No data")).toBeTruthy()
    const [url] = dataRequests(fetch)
    expect(url?.searchParams.get("from")).toBe("2026-11-06T10:00:00Z")
    expect(url?.searchParams.get("to")).toBe("2026-11-06T11:00:00Z")
  })

  it("names the datasource, says which window the block is pinned to and credits the person who added it", async () => {
    stubData(() => json({ frames: [], meta: {} }, 200))
    renderBlock(readable)

    expect(
      await screen.findByText(
        `Prometheus production, Pinned to ${describeRange("2026-11-06T10:00:00Z", "2026-11-06T11:00:00Z")}`
      )
    ).toBeTruthy()
    expect(screen.getByText("Added by Priya")).toBeTruthy()
  })

  it("says which values the panel runs with when the block carries variables", async () => {
    stubData(() => json({ frames: [], meta: {} }, 200))
    renderBlock(
      panelBlock({
        panel: {
          type: "timeseries",
          title: "Errors",
          datasource: "prom-prod",
          queries: [{ query: 'up{service=~"$service"}' }],
        },
        variables: [
          {
            name: "service",
            type: "custom",
            multi: true,
            options: [],
            default: ["checkout", "cart"],
          },
          { name: "env", type: "custom", options: [], default: "$__all" },
        ],
      })
    )

    expect(
      await screen.findByText(
        "Prometheus production, Last hour, live, with service=checkout,cart, env=All"
      )
    ).toBeTruthy()
  })

  it("shows the server's sentence when the data route refuses the caller", async () => {
    const fetch = stubData(() =>
      json(
        {
          error: "Panel data needs a signed-in user.",
          code: "user_session_required",
        },
        403
      )
    )
    renderBlock(readable)

    expect(
      await screen.findByText("Panel data needs a signed-in user.")
    ).toBeTruthy()
    expect(dataRequests(fetch)).toHaveLength(1)
  })

  it("says the block could not be read and asks for no data when its body is malformed", async () => {
    const fetch = stubData(() => json({ frames: [], meta: {} }, 200))
    renderBlock(panelBlock({ panel: "not a panel" }))

    expect(
      await screen.findByText("This block could not be read.")
    ).toBeTruthy()
    expect(dataRequests(fetch)).toHaveLength(0)
  })

  it("offers the dashboard panel's own tools and none of the dashboard's actions", async () => {
    stubData(() => json({ frames: [], meta: {} }, 200))
    const inspect = { inspectedId: null, open: vi.fn(), publish: vi.fn() }
    const { user } = renderBlock(readable, inspect)

    await user.click(
      await screen.findByRole("button", { name: "Panel menu: Error rate" })
    )
    const items = await screen.findAllByRole("menuitem")
    expect(items).toHaveLength(5)
    for (const name of [
      "View as table",
      "Inspect",
      "Open in Metrics",
      "Download CSV",
      "Save as PNG",
    ]) {
      expect(screen.getByRole("menuitem", { name })).toBeTruthy()
    }

    for (const item of items) {
      expect(item.querySelector('[data-slot="dropdown-menu-shortcut"]')).toBe(
        null
      )
    }

    await user.click(screen.getByRole("menuitem", { name: "Inspect" }))
    expect(inspect.open).toHaveBeenCalledWith("b1")
  })

  it("opens a logs block in Logs over the block's range", async () => {
    stubData(() => json({ frames: [], meta: {} }, 200))
    const { user } = renderBlock(
      panelBlock({
        panel: {
          type: "logs",
          title: "Checkout logs",
          datasource: "loki-prod",
          queries: [{ query: '{service="checkout"}' }],
        },
        from: "2026-11-06T10:00:00Z",
        to: "2026-11-06T11:00:00Z",
      })
    )

    await user.click(
      await screen.findByRole("button", { name: "Panel menu: Checkout logs" })
    )
    expect(
      await screen.findByRole("menuitem", { name: /^Open in Logs/ })
    ).toBeTruthy()
  })

  it("runs the panel with a value picked from the block's own options", async () => {
    const fetch = stubData(() => json({ frames: [], meta: {} }, 200))
    const { user } = renderBlock(
      panelBlock({
        panel: {
          type: "timeseries",
          title: "Errors",
          datasource: "prom-prod",
          queries: [{ query: 'up{env="$env"}' }],
        },
        variables: [
          {
            name: "env",
            type: "custom",
            options: ["prod", "staging"],
            default: "prod",
          },
        ],
      })
    )

    await user.click(await screen.findByRole("combobox", { name: "env" }))
    await user.click(await screen.findByRole("option", { name: "staging" }))

    await waitFor(() =>
      expect(
        dataRequests(fetch).at(-1)?.searchParams.getAll("var-env")
      ).toEqual(["staging"])
    )
    expect(dataRequests(fetch)[0]?.searchParams.getAll("var-env")).toEqual([
      "prod",
    ])
  })

  it("follows the page's range when the block has none of its own", async () => {
    const fetch = stubData(() => json({ frames: [], meta: {} }, 200))
    const page = { from: "2026-11-20T18:26:00Z", to: "2026-11-20T19:26:00Z" }
    renderBlock(following, null, page)

    expect(
      await screen.findByText(
        `Prometheus production, ${describeRange(page.from, page.to)}`
      )
    ).toBeTruthy()
    await waitFor(() => expect(dataRequests(fetch)).toHaveLength(1))
    const [url] = dataRequests(fetch)
    expect(url?.searchParams.get("from")).toBe(page.from)
    expect(url?.searchParams.get("to")).toBe(page.to)
  })

  it("shows the last hour live when neither the page nor the block has a range", async () => {
    const fetch = stubData(() => json({ frames: [], meta: {} }, 200))
    renderBlock(following)

    expect(
      await screen.findByText("Prometheus production, Last hour, live")
    ).toBeTruthy()
    await waitFor(() => expect(dataRequests(fetch)).toHaveLength(1))
    const [url] = dataRequests(fetch)
    expect(url?.searchParams.get("from")).toBe("now-1h")
    expect(url?.searchParams.get("to")).toBe("now")
  })

  it("pins a following block to the window it shows now", async () => {
    const fetch = stubData(() => json({ frames: [], meta: {} }, 200))
    const page = { from: "2026-11-20T18:26:00Z", to: "2026-11-20T19:26:00Z" }
    const { user } = renderBlock(following, null, page)

    await user.click(
      await screen.findByRole("button", { name: "Panel menu: Error rate" })
    )
    expect(
      screen.queryByRole("menuitem", { name: "Follow the page" })
    ).toBeNull()
    expect(
      screen.getByRole("menuitem", { name: "Pin to a window" })
    ).toBeTruthy()
    await user.click(
      screen.getByRole("menuitem", { name: "Pin to this window" })
    )

    await waitFor(async () =>
      expect(await blockWrites(fetch)).toEqual([
        {
          version: 3,
          body: {
            panel: errorRate,
            from: "2026-11-20T18:26:00.000Z",
            to: "2026-11-20T19:26:00.000Z",
          },
        },
      ])
    )
  })

  it("lets a pinned block follow the page again", async () => {
    const fetch = stubData(() => json({ frames: [], meta: {} }, 200))
    const { user } = renderBlock(
      panelBlock(readable.body, ["update"]),
      null,
      noPage
    )

    await user.click(
      await screen.findByRole("button", { name: "Panel menu: Error rate" })
    )
    expect(
      screen.queryByRole("menuitem", { name: "Pin to this window" })
    ).toBeNull()
    await user.click(screen.getByRole("menuitem", { name: "Follow the page" }))

    await waitFor(async () =>
      expect(await blockWrites(fetch)).toEqual([
        { version: 3, follow_page: true },
      ])
    )
  })

  it("pins a block to a window chosen in the range control", async () => {
    const fetch = stubData(() => json({ frames: [], meta: {} }, 200))
    const { user } = renderBlock(following)

    await user.click(
      await screen.findByRole("button", { name: "Panel menu: Error rate" })
    )
    await user.click(screen.getByRole("menuitem", { name: "Pin to a window" }))
    const from = await screen.findByLabelText("From")
    const to = screen.getByLabelText("To")
    await user.clear(from)
    await user.type(from, "2026-11-20T10:00:00Z")
    await user.clear(to)
    await user.type(to, "2026-11-20T11:00:00Z")
    await user.click(screen.getByRole("button", { name: "Apply" }))

    await waitFor(async () =>
      expect(await blockWrites(fetch)).toEqual([
        {
          version: 3,
          body: {
            panel: errorRate,
            from: "2026-11-20T10:00:00.000Z",
            to: "2026-11-20T11:00:00.000Z",
          },
        },
      ])
    )
  })

  it("shows a free-entry variable's stored value without a field to type into", async () => {
    const fetch = stubData(() => json({ frames: [], meta: {} }, 200))
    renderBlock(
      panelBlock({
        panel: {
          type: "timeseries",
          title: "Errors",
          datasource: "prom-prod",
          queries: [{ query: 'up{service="$svc"}' }],
        },
        variables: [{ name: "svc", type: "textbox", default: "checkout" }],
      })
    )

    expect(await screen.findByText("checkout")).toBeTruthy()
    expect(screen.queryByRole("textbox")).toBeNull()
    await waitFor(() => expect(dataRequests(fetch)).toHaveLength(1))
    expect(dataRequests(fetch)[0]?.searchParams.getAll("var-svc")).toEqual([
      "checkout",
    ])
  })
})
