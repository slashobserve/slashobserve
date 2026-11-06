import { screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { DatasourceStatus } from "@/datasources/queries"
import { createFieldResolver } from "@/panels/field-config"
import type { LogDetail, LogEntry } from "@/panels/logs/logs-model"
import type { LogSelection } from "@/panels/logs/logs-panel"
import { renderWithProviders } from "@/test/render"

import { ContextTab } from "./context-panel"
import type { LogContext } from "./context-query"

const anchorNs = "1762438796641000000"

const resolver = createFieldResolver({
  id: "logs",
  type: "logs",
  title: "Logs",
  datasource: "ds",
  gridPos: { x: 0, y: 0, w: 24, h: 10 },
  queries: [{ query: "" }],
})

function datasource(type: string): DatasourceStatus {
  return {
    uid: "ds",
    type,
    name: "ds",
    healthy: true,
    checked_at: "2026-11-07T12:00:00Z",
    dashboard_count: 0,
  } as DatasourceStatus
}

function selection(
  details: [string, string][],
  streamLabels: Record<string, string>,
  time: number | null = 1_762_438_796_641
): LogSelection {
  const entry: LogEntry = {
    key: "k",
    time,
    level: null,
    message: "acquirer-b returned 503",
    labels: streamLabels,
    repeats: 1,
    details: details.map(([name, text]): LogDetail => ({ name, text })),
  }
  return { entry, streamLabels, fields: resolver }
}

function side(truncated: boolean, ns: string[] = []): LogContext["before"] {
  return {
    truncated,
    frames:
      ns.length === 0
        ? []
        : [
            {
              ref: "A",
              fields: [
                { name: "TimeNs", type: "string", values: ns },
                { name: "Body", type: "string", values: ns },
              ],
            },
          ],
  }
}

let bodies: Record<string, unknown>[] = []
let answer: LogContext

beforeEach(() => {
  bodies = []
  vi.stubGlobal("fetch", async (request: Request) => {
    bodies.push((await request.json()) as Record<string, unknown>)
    return new Response(JSON.stringify(answer), {
      status: 200,
      headers: { "content-type": "application/json" },
    })
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

const lokiSelection = () =>
  selection([["TimeNs", anchorNs]], { service_name: "checkout" })

describe("ContextTab", () => {
  it("asks once for 50 lines each side within an hour, keyed on the line's nanosecond time", async () => {
    answer = { anchor: anchorNs, before: side(false), after: side(false) }
    renderWithProviders(
      <ContextTab
        datasource={datasource("loki")}
        anchor={lokiSelection()}
        indexedLabels={["service_name"]}
      />
    )
    expect(
      await screen.findByText("No more lines within 1 hour before.")
    ).toBeTruthy()
    expect(screen.getByText("No more lines within 1 hour after.")).toBeTruthy()
    expect(bodies).toEqual([
      {
        query: '{service_name="checkout"}',
        anchor: anchorNs,
        before: 50,
        after: 50,
        window_ms: 3_600_000,
      },
    ])
    expect(
      screen.getByRole("heading", { name: "Surrounding lines" })
    ).toBeTruthy()
  })

  it("loads 10 more on the side that says more lines exist", async () => {
    answer = {
      anchor: anchorNs,
      before: side(true, ["1"]),
      after: side(false, ["2"]),
    }
    const { user } = renderWithProviders(
      <ContextTab
        datasource={datasource("loki")}
        anchor={lokiSelection()}
        indexedLabels={undefined}
      />
    )
    const older = await screen.findByRole("button", { name: "Load 10 older" })
    expect(screen.queryByRole("button", { name: "Load 10 newer" })).toBeNull()
    await user.click(older)
    await vi.waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies[1]).toMatchObject({ before: 60, after: 50 })
  })

  it("widens the window from the Window control", async () => {
    answer = { anchor: anchorNs, before: side(false), after: side(false) }
    const { user } = renderWithProviders(
      <ContextTab
        datasource={datasource("loki")}
        anchor={lokiSelection()}
        indexedLabels={undefined}
      />
    )
    await user.click(await screen.findByRole("combobox", { name: "Window" }))
    await user.click(await screen.findByRole("option", { name: "24 hours" }))
    await vi.waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies[1]).toMatchObject({ window_ms: 86_400_000 })
    expect(
      await screen.findByText("No more lines within 24 hours before.")
    ).toBeTruthy()
  })

  it("says why a line without stream labels has no context and sends nothing", async () => {
    renderWithProviders(
      <ContextTab
        datasource={datasource("loki")}
        anchor={selection([["TimeNs", anchorNs]], { trace_id: "abc" })}
        indexedLabels={undefined}
      />
    )
    expect(
      await screen.findByText("This line has no stream labels to search by.")
    ).toBeTruthy()
    expect(bodies).toEqual([])
  })

  it("titles the section for documents and notes millisecond precision on a date field", async () => {
    answer = {
      anchor: anchorNs,
      before: side(false, ["1762438796640000000"]),
      after: side(false),
    }
    renderWithProviders(
      <ContextTab
        datasource={datasource("elasticsearch")}
        anchor={selection([["TimeNs", anchorNs]], {})}
        indexedLabels={undefined}
      />
    )
    expect(
      await screen.findByText(
        "On a date field, context is exact to the millisecond."
      )
    ).toBeTruthy()
    expect(
      screen.getByRole("heading", { name: "Surrounding documents" })
    ).toBeTruthy()
    expect(bodies[0]).toMatchObject({ query: "*" })
  })

  it("leaves the millisecond note out when the lines carry nanoseconds", async () => {
    answer = {
      anchor: anchorNs,
      before: side(false, ["1762438796640123456"]),
      after: side(false),
    }
    renderWithProviders(
      <ContextTab
        datasource={datasource("opensearch")}
        anchor={selection([["TimeNs", "1762438796641123456"]], {})}
        indexedLabels={undefined}
      />
    )
    await screen.findByText("No more lines within 1 hour after.")
    expect(screen.queryByText(/exact to the millisecond/)).toBeNull()
  })
})
