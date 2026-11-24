import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { TurnFailure } from "./turn-failure"

const base = {
  id: "a77cc709-1b2c-4d3e-8f4a-5b6c7d8e9f01",
  intent: "apply" as const,
  base_version: null,
  comment_id: null,
  dashboard_id: null,
  error: "the run was canceled",
  continues_id: null,
  continued_by: null,
  retried_by: null,
  build: null,
  build_saved_at: null,
  allowed_actions: ["retry" as const],
}

function renderFailure(failureCode: string, extra: object = {}) {
  return renderWithProviders(
    <TurnFailure
      request={{ ...base, failure_code: failureCode, ...extra }}
      onRetried={vi.fn()}
    />,
    { route: { path: "/", url: "/" } }
  )
}

describe("TurnFailure", () => {
  it("shows a stopped run as a calm status, once", async () => {
    renderFailure("canceled")
    expect(await screen.findByRole("status")).toHaveTextContent(
      "You stopped this run"
    )
    expect(screen.queryByRole("alert")).toBeNull()
    expect(screen.queryByText(/could not be built/i)).toBeNull()
    expect(screen.queryAllByText(/was canceled/i)).toHaveLength(0)
    expect(
      screen.getByRole("button", { name: "Try again" })
    ).toBeInTheDocument()
  })

  it("offers saving or continuing only while a build is left", async () => {
    renderWithProviders(
      <TurnFailure
        request={{
          ...base,
          failure_code: "canceled",
          allowed_actions: ["retry", "continue", "keep_build"],
        }}
        onRetried={vi.fn()}
      />,
      { route: { path: "/", url: "/" } }
    )
    expect(
      await screen.findByText(/Save what was built, continue it/)
    ).toBeInTheDocument()
  })

  it("says nothing was built when the run stopped before a panel", async () => {
    renderFailure("canceled")
    expect(await screen.findByText("Try again.")).toBeInTheDocument()
    expect(screen.queryByText(/Save what was built/)).toBeNull()
  })

  it("points to the retry and offers no second one", async () => {
    renderWithProviders(
      <TurnFailure
        request={{
          ...base,
          failure_code: "canceled",
          retried_by: "b11cc709-1b2c-4d3e-8f4a-5b6c7d8e9f02",
          allowed_actions: [],
        }}
        onRetried={vi.fn()}
      />,
      { route: { path: "/", url: "/" } }
    )
    expect(
      await screen.findByRole("link", { name: "Tried again in a new run" })
    ).toHaveAttribute("href", "/c/b11cc709-1b2c-4d3e-8f4a-5b6c7d8e9f02")
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull()
  })

  it("keeps a real failure as an alert with its reason", async () => {
    renderFailure("no_progress")
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The dashboard could not be built"
    )
  })

  it("titles a failed question as a question and prints the reason once", async () => {
    renderFailure("no_progress", {
      intent: "ask",
      kind: "build",
      error: "the agent stopped making progress",
    })
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The question could not be answered"
    )
    expect(screen.queryByText(/could not be built/i)).toBeNull()
    expect(
      screen.getAllByText("The agent stopped making progress.")
    ).toHaveLength(1)
  })

  it("titles a question the same for a viewer with no actions", async () => {
    renderFailure("no_progress", {
      intent: "ask",
      kind: "build",
      allowed_actions: [],
    })
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The question could not be answered"
    )
  })

  it("keeps the dashboard title for an ask that built panels, whatever the viewer may do", async () => {
    renderFailure("no_progress", {
      intent: "ask",
      kind: "build",
      allowed_actions: [],
      build: {
        saves: 2,
        panels: 2,
        planned_panels: 4,
        title: null,
        updated_at: "2026-11-25T10:00:00Z",
      },
    })
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The dashboard could not be built"
    )
  })

  it("keeps the dashboard title once the build moved to a continued run", async () => {
    renderFailure("no_progress", {
      intent: "ask",
      kind: "build",
      allowed_actions: [],
      continued_by: "b11cc709-1b2c-4d3e-8f4a-5b6c7d8e9f02",
    })
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The dashboard could not be built"
    )
  })

  it("keeps a reason that adds to the explanation", async () => {
    renderFailure("no_progress", {
      error: "Stopped: the model kept reading after 12 calls without a save",
    })
    expect(
      await screen.findByText(
        "Stopped: the model kept reading after 12 calls without a save."
      )
    ).toBeInTheDocument()
  })
})
