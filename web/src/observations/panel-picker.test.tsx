import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import { PanelPicker } from "./panel-picker"
import type { Observation } from "./queries"

const observation = {
  id: "o1",
  title: "Checkout slow",
  from: "2026-11-12T08:00:00Z",
  to: "2026-11-12T09:00:00Z",
} as unknown as Observation

const replies: Record<string, unknown> = {
  "GET /api/v1/dashboards": {
    dashboards: [
      { id: "d1", title: "Checkout", folder: { id: "f1", title: "Shop" } },
    ],
    next_cursor: null,
  },
  "GET /api/v1/dashboards/d1": {
    version: {
      version: 2,
      spec: {
        specVersion: "0",
        title: "Checkout",
        time: { from: "now-1h", to: "now" },
        panels: [
          {
            id: "errors",
            type: "timeseries",
            title: "Errors",
            datasource: "prom",
            gridPos: { x: 0, y: 0, w: 12, h: 8 },
            queries: [{ query: "up" }],
          },
        ],
      },
    },
  },
}

let added: unknown[] = []

beforeEach(() => {
  added = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      if (
        request.method === "POST" &&
        url.pathname === "/api/v1/observations/o1/blocks"
      ) {
        added.push(await request.json())
        return Response.json({ block: { id: "b9" } }, { status: 201 })
      }
      const body = replies[`${request.method} ${url.pathname}`]
      if (body === undefined) return new Response(null, { status: 599 })
      return Response.json(body)
    })
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

describe("PanelPicker", () => {
  it("adds the chosen panel as a block that follows the page's range", async () => {
    const host = document.createElement("div")
    document.body.append(host)
    const onPicked = vi.fn()
    const { user } = renderWithProviders(
      <PanelHostContext value={host}>
        <PanelPicker
          observation={observation}
          verb="Add"
          onPicked={onPicked}
          onClose={() => {}}
          trigger={{ current: null }}
        />
      </PanelHostContext>,
      { route: { path: "/", url: "/" } }
    )

    await user.click(await screen.findByRole("combobox", { name: "Dashboard" }))
    await user.click(await screen.findByRole("option", { name: /Checkout/ }))
    await user.click(await screen.findByRole("combobox", { name: "Panel" }))
    await user.click(await screen.findByRole("option", { name: /Errors/ }))
    await user.click(screen.getByRole("button", { name: "Add" }))

    await waitFor(() => expect(onPicked).toHaveBeenCalledWith("b9"))
    expect(added).toEqual([
      {
        kind: "panel",
        body: {
          panel: {
            type: "timeseries",
            title: "Errors",
            datasource: "prom",
            queries: [{ query: "up" }],
          },
        },
      },
    ])
  })
})
