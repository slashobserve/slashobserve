import { screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { OriginFields } from "@/observe/request-origin"
import { renderWithProviders } from "@/test/render"

import {
  opensBuildPage,
  RequestTurn,
  workingText,
  type TurnSummary,
} from "./request-turn"

const fetchMock = vi.hoisted(() => vi.fn())

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "viewer-1" } }),
}))

const failed: TurnSummary = {
  id: "a77cc709-1b2c-4d3e-8f4a-5b6c7d8e9f01",
  intent: "apply",
  investigation: false,
  base_version: 1,
  comment_id: null,
  prompt: "Add a latency panel",
  status: "failed",
  state: "failed",
  result_kind: "none",
  allowed_actions: ["retry"],
  dashboard_id: null,
  result_version: null,
  error: "the agent stopped",
  panel_id: null,
  requested_by: { id: "someone-else", name: "Grace" },
  created_at: "2026-11-06T10:00:00Z",
  started_at: "2026-11-06T10:00:01Z",
  finished_at: "2026-11-06T10:01:00Z",
  failure_code: "no_progress",
  build: null,
  build_saved_at: null,
  continues_id: null,
  continued_by: null,
  retried_by: null,
  continue_note: null,
}

function renderTurn(summary: TurnSummary) {
  return renderWithProviders(
    <RequestTurn summary={summary} loadSteps={false} onRetried={vi.fn()} />,
    { route: { path: "/", url: "/" } }
  )
}

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal("fetch", fetchMock)
  return () => vi.unstubAllGlobals()
})

describe("RequestTurn", () => {
  it("offers Try again to a failed turn from its summary without loading the request", async () => {
    renderTurn(failed)
    expect(
      await screen.findByRole("button", { name: "Try again" })
    ).toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it("offers no Try again when the summary allows none", async () => {
    renderTurn({ ...failed, allowed_actions: [] })
    expect(await screen.findByRole("alert")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe("RequestTurn with a build saved before the run finished", () => {
  const saved: TurnSummary = {
    ...failed,
    requested_by: { id: "viewer-1", name: "Ada" },
    dashboard_id: "d1",
    result_kind: "partial_version",
    result_version: 1,
    allowed_actions: [],
    build: null,
  }

  beforeEach(() => {
    fetchMock.mockImplementation(
      async () =>
        new Response(
          JSON.stringify({
            dashboard: { id: "d1" },
            version: {
              version: 1,
              summary: "5 of 8 panels",
              spec: {
                specVersion: "0",
                title: "Shop",
                time: { from: "now-1h", to: "now" },
                panels: [],
              },
            },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } }
        )
    )
  })

  it("offers Add the missing panels while it is the latest turn", async () => {
    renderWithProviders(
      <RequestTurn summary={saved} loadSteps onRetried={vi.fn()} latest />,
      { route: { path: "/", url: "/" } }
    )
    expect(
      await screen.findByRole("button", { name: "Add the missing panels" })
    ).toBeInTheDocument()
  })

  it("drops the button once a later turn exists", async () => {
    renderWithProviders(
      <RequestTurn
        summary={saved}
        loadSteps
        onRetried={vi.fn()}
        latest={false}
      />,
      { route: { path: "/", url: "/" } }
    )
    expect(
      await screen.findByText(/saved before the run finished/)
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Add the missing panels" })
    ).toBeNull()
  })
})

describe("RequestTurn while a run settles", () => {
  const answered: TurnSummary = {
    ...failed,
    intent: "apply",
    base_version: null,
    comment_id: null,
    status: "succeeded",
    state: "succeeded",
    result_kind: "observation",
    allowed_actions: [],
    failure_code: null,
    error: null,
  }

  function serve(detail: object) {
    fetchMock.mockImplementation(
      async () =>
        new Response(JSON.stringify({ request: detail }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
    )
  }

  it("does not describe a stale running read as a run with no steps", async () => {
    serve({
      ...answered,
      status: "running",
      state: "running",
      steps: [],
      finding: null,
    })
    renderTurn(answered)
    expect(await screen.findByText("Show steps")).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    expect(screen.queryByText("No steps recorded")).toBeNull()
    expect(screen.queryByText(/finished without a dashboard/)).toBeNull()
  })

  it("waits for the answer instead of saying nothing was produced", async () => {
    serve({ ...answered, steps: [{ id: "s1" }], finding: null })
    renderTurn(answered)
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    expect(screen.queryByText(/finished without a dashboard/)).toBeNull()
  })
})

const building: OriginFields &
  Pick<TurnSummary, "status" | "dashboard_id" | "build"> = {
  intent: "apply",
  base_version: null,
  comment_id: null,
  status: "running",
  dashboard_id: null,
  build: {
    saves: 1,
    panels: 1,
    planned_panels: 4,
    title: "Checkout",
    updated_at: "2026-11-06T10:00:30Z",
  },
}

describe("opensBuildPage", () => {
  it("opens the canvas once a new dashboard has a saved panel", () => {
    expect(
      opensBuildPage({ request: building, own: true, onChatPage: true })
    ).toBe(true)
  })

  it("keeps a question on the chat page until a build exists", () => {
    expect(
      opensBuildPage({
        request: { ...building, build: null },
        own: true,
        onChatPage: true,
      })
    ).toBe(false)
  })

  it("stays put for a finished run, another person's run and the pane", () => {
    const base = { request: building, own: true, onChatPage: true }
    expect(
      opensBuildPage({ ...base, request: { ...building, status: "failed" } })
    ).toBe(false)
    expect(opensBuildPage({ ...base, own: false })).toBe(false)
    expect(opensBuildPage({ ...base, onChatPage: false })).toBe(false)
  })

  it("never moves a revision of an existing dashboard", () => {
    expect(
      opensBuildPage({
        request: { ...building, dashboard_id: "d1" },
        own: true,
        onChatPage: true,
      })
    ).toBe(false)
  })
})

describe("workingText", () => {
  it("does not promise a dashboard before one is being built", () => {
    expect(workingText({ dashboard_id: null, build: null }, undefined)).toBe(
      "Working on your request"
    )
  })

  it("names the dashboard being built or revised", () => {
    expect(
      workingText({ dashboard_id: null, build: building.build }, undefined)
    ).toBe("Building the dashboard")
    expect(workingText({ dashboard_id: "d1", build: null }, undefined)).toBe(
      "Revising the dashboard"
    )
    expect(workingText({ dashboard_id: "d1", build: null }, "Checkout")).toBe(
      "Revising Checkout"
    )
  })
})
