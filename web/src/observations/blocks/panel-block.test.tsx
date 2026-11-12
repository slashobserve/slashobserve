import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { datasourceKeys } from "@/api/keys"
import type { Block } from "@/observations/queries"
import { renderWithProviders, testQueryClient } from "@/test/render"

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

function stubData(response: () => Response) {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(async () =>
    response()
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

function renderBlock(block: Block) {
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
  return renderWithProviders(<PanelBlock block={block} height={240} />, {
    queryClient,
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("PanelBlock", () => {
  it("asks the block data route for the block's own range", async () => {
    const fetch = stubData(() => json({ frames: [], meta: {} }, 200))
    renderBlock(readable)

    expect(await screen.findByText("No data")).toBeTruthy()
    const url = new URL(fetch.mock.calls[0][0].url)
    expect(url.pathname).toBe("/api/v1/blocks/b1/data")
    expect(url.searchParams.get("from")).toBe("2026-11-06T10:00:00Z")
    expect(url.searchParams.get("to")).toBe("2026-11-06T11:00:00Z")
  })

  it("names the datasource, says the data is live and credits the person who added it", async () => {
    stubData(() => json({ frames: [], meta: {} }, 200))
    renderBlock(readable)

    expect(await screen.findByText("Prometheus production, live")).toBeTruthy()
    expect(screen.getByText("Added by Priya")).toBeTruthy()
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
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it("says the block could not be read and asks for no data when its body is malformed", () => {
    const fetch = stubData(() => json({ frames: [], meta: {} }, 200))
    renderBlock(panelBlock({ panel: "not a panel" }))

    expect(screen.getByText("This block could not be read.")).toBeTruthy()
    expect(fetch).not.toHaveBeenCalled()
  })
})
