import { screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { RequestTurn, type TurnSummary } from "./request-turn"

const fetchMock = vi.hoisted(() => vi.fn())

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "viewer-1" } }),
}))

const failed: TurnSummary = {
  id: "a77cc709-1b2c-4d3e-8f4a-5b6c7d8e9f01",
  source: "prompt",
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
