import { cleanup, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { datasourceKeys } from "@/api/keys"
import type { Block } from "@/observations/queries"
import { renderWithProviders, testQueryClient } from "@/test/render"

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

function panelBlock(body: Record<string, unknown>): Block {
  return {
    id: "b1",
    kind: "panel",
    body,
    created_by: person,
  } as unknown as Block
}

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

function dataRequests(fetch: ReturnType<typeof stubData>): URL[] {
  return fetch.mock.calls
    .map(([request]) => new URL(request.url))
    .filter((url) => url.pathname === blockData)
}

function renderBlock(block: Block, inspect: BlockInspect | null = null) {
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
    <BlockInspectContext value={inspect}>
      <PanelBlock block={block} height={240} />
    </BlockInspectContext>,
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
    expect(url.searchParams.get("from")).toBe("2026-11-06T10:00:00Z")
    expect(url.searchParams.get("to")).toBe("2026-11-06T11:00:00Z")
  })

  it("names the datasource, says the data is live and credits the person who added it", async () => {
    stubData(() => json({ frames: [], meta: {} }, 200))
    renderBlock(readable)

    expect(await screen.findByText("Prometheus production, live")).toBeTruthy()
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
        "Prometheus production, live, with service=checkout,cart, env=All"
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
    expect(items).toHaveLength(4)
    for (const name of [
      "View as table",
      "Inspect",
      "Download CSV",
      "Save as PNG",
    ]) {
      expect(screen.getByRole("menuitem", { name })).toBeTruthy()
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
    expect(dataRequests(fetch)[0].searchParams.getAll("var-env")).toEqual([
      "prod",
    ])
  })
})
